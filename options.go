// Package permission
//
// @author: xwc1125
package permission

import (
	"fmt"
	"github.com/chain5j/chain5j-protocol/protocol"
	"github.com/chain5j/logger"
)

type Factory struct {
	log         logger.Logger
	networkID   uint64
	p2pService  protocol.P2PService
	blockRW     protocol.BlockReadWriter
	broadcaster protocol.Broadcaster
}

type option func(f *protocolManager) error

func apply(f *protocolManager, opts ...option) error {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(f); err != nil {
			return fmt.Errorf("option apply err:%v", err)
		}
	}
	return nil
}

func WithP2PService(p2pService protocol.P2PService) option {
	return func(f *protocolManager) error {
		f.p2pService = p2pService
		return nil
	}
}

func WithBlockRW(blockRW protocol.BlockReadWriter) option {
	return func(f *protocolManager) error {
		f.blockRW = blockRW
		return nil
	}
}

func WithBroadcaster(broadcaster protocol.Broadcaster) option {
	return func(f *protocolManager) error {
		f.broadcaster = broadcaster
		return nil
	}
}
