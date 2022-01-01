// Package permission
//
// @author: xwc1125
package permission

import (
	"context"
	"errors"
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/event"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/chain5j-protocol/models/eventtype"
	"github.com/chain5j/chain5j-protocol/protocol"
	"github.com/chain5j/logger"
	"github.com/hashicorp/golang-lru"
)

const (
	chainHeadChanSize = 64
	p2pBlockChanSize  = 128
	inMemoryBlock     = 16
)

// protocolManager 协议管理
type protocolManager struct {
	log    logger.Logger
	ctx    context.Context
	cancel context.CancelFunc

	netWorkId   uint64
	p2pService  protocol.P2PService
	blockRW     protocol.BlockReadWriter
	broadcaster protocol.Broadcaster

	blockEventSub event.Subscription            // 本地区块事件
	chainEventCh  chan eventtype.ChainHeadEvent // 本地区块

	p2pBlockSub event.Subscription
	p2pBlockCh  chan *models.P2PMessage

	knownBlock *lru.ARCCache // 已知区块，不再进行入库操作

	feed  event.Feed
	scope event.SubscriptionScope
}

func NewProtocolManager(rootCtx context.Context, networkID uint64, opts ...option) (protocol.Handshake, error) {
	ctx, cancel := context.WithCancel(rootCtx)
	p := &protocolManager{
		log:       logger.New("permission"),
		ctx:       ctx,
		cancel:    cancel,
		netWorkId: networkID,
	}
	if err := apply(p, opts...); err != nil {
		p.log.Error("apply is error", "err", err)
		return nil, err
	}
	p.chainEventCh = make(chan eventtype.ChainHeadEvent, chainHeadChanSize)
	p.blockEventSub = p.blockRW.SubscribeChainHeadEvent(p.chainEventCh)

	p.p2pBlockCh = make(chan *models.P2PMessage, p2pBlockChanSize)
	p.p2pBlockSub = p.p2pService.SubscribeMsg(models.BlockMsg, p.p2pBlockCh)

	p.knownBlock, _ = lru.NewARC(inMemoryBlock)

	return p, nil
}

func (pm *protocolManager) Start() error {
	go pm.listen()
	go pm.handleBlockMsg()
	return nil
}

func (pm *protocolManager) handleBlockMsg() {
	for {
		select {
		case msg := <-pm.p2pBlockCh:
			var block models.Block
			if err := codec.Coder().Decode(msg.Data, &block); err != nil {
				pm.log.Error("protocoManager decode block msg error", "error", err)
			}
			pm.log.Debug("Handle Broadcast Block", "blockHeight", block.Height())
			if _, ok := pm.knownBlock.Get(block.Hash()); ok {
				continue
			}
			pm.knownBlock.Add(block.Hash(), true)
			// TODO mark peer know block
			pm.blockRW.ProcessBlock(&block, true)

		case event := <-pm.chainEventCh:
			pm.log.Debug("Broadcast Block", "blockHeight", event.Block.Height())
			payload, _ := codec.Coder().Encode(event.Block)
			pm.knownBlock.Add(event.Block.Hash(), true)
			pm.broadcaster.Broadcast(nil, models.BlockMsg, payload)

		case err := <-pm.blockEventSub.Err():
			if err != nil {
				pm.log.Error("protocoManagger subscribe block event error", "error", err)
			}
			return
		case err := <-pm.p2pBlockSub.Err():
			pm.log.Error("protocoManagger p2p block msg error", "error", err)
			return
		}
	}
}

func (pm *protocolManager) listen() {
	// handshake需要校验
	handshakePeerCh := make(chan models.P2PID)
	handshakePeerSub := pm.p2pService.SubscribeHandshakePeer(handshakePeerCh)

	// handshake响应
	handMsgCh := make(chan *models.P2PMessage, 1)
	handMsgSub := pm.p2pService.SubscribeMsg(models.MsgTypeHandshake, handMsgCh)

	for {
		select {
		case ch := <-handshakePeerCh:
			pm.RequestHandshake(ch)
		case err := <-handshakePeerSub.Err():
			pm.log.Error("handshakePeerSub", "err", err)
		case ch := <-handMsgCh:
			var msg *models.HandshakeMsg
			data := ch.Data
			err := codec.Coder().Decode(data, &msg)
			if err != nil {
				pm.log.Error("listen decode handshakeMsg", "err", err)
				break
			}
			pm.respondHandshake(ch.Peer, msg)
		case err := <-handMsgSub.Err():
			pm.log.Error("handMsgSub", "err", err)
		case <-pm.ctx.Done():
			return
		}
	}
}

func (pm *protocolManager) Stop() error {
	pm.cancel()
	pm.scope.Close()
	return nil
}

// RequestHandshake 对p2p进行握手协议处理
// 先注册p2p，然后通过chan来进行协议确认
func (pm *protocolManager) RequestHandshake(id models.P2PID) error {
	pm.log.Debug("requestHandshake start")
	currentBlock := pm.blockRW.CurrentBlock()
	genesisBlock := pm.blockRW.GetBlockByNumber(0)

	msg := models.HandshakeMsg{
		// ProtocolVersion: 0,
		NetworkId:          pm.netWorkId,
		CurrentBlockHash:   currentBlock.Hash(),
		CurrentBlockHeight: currentBlock.Height(),
		GenesisBlockHash:   genesisBlock.Hash(),
	}
	bytes, err := codec.Coder().Encode(&msg)
	if err != nil {
		pm.log.Error("requestHandshake", "err", err)
		return err
	}
	return pm.p2pService.Send(id, &models.P2PMessage{
		Type: models.MsgTypeHandshake,
		Peer: "",
		Data: bytes,
	})
}

// respondHandshake 响应握手协议
func (pm *protocolManager) respondHandshake(peer models.P2PID, msg *models.HandshakeMsg) error {
	pm.log.Debug("respondHandshake start")
	if pm.netWorkId != msg.NetworkId {
		pm.log.Debug("listen handshake err,networkId is diff", "self", pm.netWorkId, "peer", msg.NetworkId)
		pm.p2pService.DropPeer(peer)
		return errors.New("listen handshake err,networkId is diff")
	}
	genesisBlock := pm.blockRW.GetBlockByNumber(0)
	if genesisBlock.Hash() != msg.GenesisBlockHash {
		pm.log.Debug("listen handshake err,genesis is diff", "self", genesisBlock.Hash(), "peer", msg.GenesisBlockHash)
		pm.p2pService.DropPeer(peer)
		return errors.New("listen handshake err,genesis is diff")
	}
	// TODO 需要存储存储对方的当前高度，用于同步使用
	// currentBlock := msg.CurrentBlock
	// _ = currentBlock

	msg.Peer = peer
	// 握手协议成功
	pm.p2pService.HandshakeSuccess(peer)

	go pm.feed.Send(msg)
	pm.broadcaster.RegisterTrustPeer(peer)
	return nil
}

// SubscribeHandshake 订阅握手协议
func (pm *protocolManager) SubscribeHandshake(msg chan *models.HandshakeMsg) event.Subscription {
	return pm.scope.Track(pm.feed.Subscribe(msg))
}
