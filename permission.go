// Package permission TODO
//
// @author: xwc1125
package permission

import (
	"errors"
	"github.com/chain5j/chain5j-pkg/database/kvstore"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/chain5j-protocol/models/permission"
	"github.com/chain5j/chain5j-protocol/protocol"
	"github.com/chain5j/logger"
	"sync"
)

// nodePermission 节点权限
type nodePermission struct {
	log logger.Logger

	lock sync.RWMutex

	db         kvstore.Database
	p2pService protocol.P2PService

	chainAccessType permission.ChainAccessType // 权限类型

	currentNodeId models.P2PID                   // 当前节点ID
	adminMap      permission.MemberInfoMap       // 管理员账户[address，不能作为nodeId]
	supervisorMap permission.MemberInfoMap       // 监管账户
	peerMap       permission.MemberInfoMap       // 节点账户
	permissionMap permission.PeerPermissionAlloc // 权限账户

}

// newPeerPermission 创建新节点权限管理
func newPeerPermission(db kvstore.Database, p2pService protocol.P2PService) (*nodePermission, error) {
	p := &nodePermission{
		log:           logger.New("permission"),
		currentNodeId: p2pService.Id(),

		db:         db,
		p2pService: p2pService,
	}

	var err error
	// type
	{
		p.chainAccessType, err = getChainAccessType(p.db)
		if err != nil {
			p.log.Error("GetType err", "err", err)
			return nil, err
		}
	}

	// admin
	{
		p.adminMap, err = getAllAdmins(p.db)
		if err != nil {
			p.log.Error("GetAdmins err", "err", err)
			return nil, err
		}
	}

	// supervisor
	{
		p.supervisorMap, err = getAllSupervisors(p.db)
		if err != nil {
			p.log.Error("GetSupervisors err", "err", err)
			return nil, err
		}
	}

	// peer
	{
		p.peerMap, err = getAllPeers(p.db)
		if err != nil {
			p.log.Error("GetPeers err", "err", err)
			return nil, err
		}
	}

	// nodePermission
	{
		p.permissionMap, err = getPeerPermissions(p.db, p.currentNodeId)
		if err != nil {
			p.log.Error("GetSupervisors err", "err", err)
			return nil, err
		}
	}
	return p, nil
}

// AddAdmin 添加管理员
func (p *nodePermission) AddAdmin(key string, info permission.MemberInfo) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.peerMap[key] = info
	return writeMemberInfoMap(p.db, PRE_ADMIN, p.peerMap)
}

// DelAdmin 删除管理员
func (p *nodePermission) DelAdmin(key string, height uint64) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	var (
		admin permission.MemberInfo
		ok    bool
	)

	if admin, ok = p.adminMap[key]; !ok {
		p.log.Debug("Admin is not exist", "key", key)
		return nil
	}
	admin.Height = height
	p.adminMap[key] = admin

	return writeMemberInfoMap(p.db, PRE_ADMIN, p.adminMap)
}

// IsAdmin 判断是否为管理员
func (p *nodePermission) IsAdmin(key string, height uint64) bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	var (
		admin permission.MemberInfo
		ok    bool
	)

	if admin, ok = p.adminMap[key]; !ok {
		p.log.Debug("Peer is not exist", "key", key)
		return false
	}
	if admin.Height != 0 && admin.Height < height {
		return false
	}

	return true
}

// AddSupervisor 添加监管账户
func (p *nodePermission) AddSupervisor(key string, info permission.MemberInfo) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.supervisorMap[key] = info
	return writeMemberInfoMap(p.db, PRE_SUPERVISOR, p.supervisorMap)
}

// GetSupervisors 获取所有的监管信息
func (p *nodePermission) GetSupervisors() (permission.MemberInfoMap, error) {
	return getAllSupervisors(p.db)
}

// DelSupervisor 删除监管
func (p *nodePermission) DelSupervisor(key string) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if _, ok := p.supervisorMap[key]; !ok {
		p.log.Debug("Supervisor is not exist", "key", key)
		return nil
	}

	delete(p.supervisorMap, key)
	return writeMemberInfoMap(p.db, PRE_SUPERVISOR, p.supervisorMap)
}

// AddPeer 添加节点
func (p *nodePermission) AddPeer(key string, info permission.MemberInfo) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.peerMap[key] = info
	return writeMemberInfoMap(p.db, PRE_PEER, p.peerMap)
}

// DelPeer 删除节点
func (p *nodePermission) DelPeer(key string, height uint64) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	var (
		peer permission.MemberInfo
		ok   bool
	)

	if peer, ok = p.peerMap[key]; !ok {
		p.log.Debug("Peer is not exist", "key", key)
		return nil
	}
	peer.Height = height
	p.peerMap[key] = peer

	return writeMemberInfoMap(p.db, PRE_PEER, p.peerMap)
}

// IsPeer 判断是否为节点
func (p *nodePermission) IsPeer(key string, height uint64) bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	var (
		peer permission.MemberInfo
		ok   bool
	)

	if peer, ok = p.peerMap[key]; !ok {
		// p.log.Debug("Peer is not exist", "key", key)
		return false
	}
	if peer.Height != 0 && peer.Height < height {
		return false
	}

	return true
}

// GetPeerPermissions 获取节点信息
func (p *nodePermission) GetPeerPermissions(peerId models.P2PID) (permission.PeerPermissionAlloc, error) {
	return getPeerPermissions(p.db, peerId)
}

// AddPermission 添加peer
func (p *nodePermission) AddPermission(peerId models.P2PID, key string, info permission.MemberInfo, r permission.RoleType) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	permissions, err := p.GetPeerPermissions(peerId)
	if err != nil {
		return err
	}
	if r == permission.COLLEAGUE {
		if peerId == p.currentNodeId {
			p.permissionMap.Colleague.Put(key, info)
		}
		colleague := permissions.Colleague
		colleague.Put(key, info)
	} else if r == permission.OBSERVER {
		if peerId == p.currentNodeId {
			p.permissionMap.Observer.Put(key, info)
		}
		observer := permissions.Observer
		observer.Put(key, info)
	} else {
		return errors.New("Err roleType")
	}
	bytes, err := permissions.Encode()
	return p.db.Put(([]byte)(PRE_NODE_PERMISSION+peerId), bytes)
}

// DelPermission 删除权限
func (p *nodePermission) DelPermission(peerId models.P2PID, key string, r permission.RoleType) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	permissions, err := p.GetPeerPermissions(peerId)
	if err != nil {
		return err
	}
	if peerId == p.currentNodeId {
		if r == permission.COLLEAGUE {
			delete(p.permissionMap.Colleague, key)
		} else if r == permission.OBSERVER {
			delete(p.permissionMap.Observer, key)
		}
	}

	if r == permission.COLLEAGUE {
		colleague := permissions.Colleague
		delete(colleague, key)
	} else if r == permission.OBSERVER {
		observer := permissions.Observer
		delete(observer, key)
	} else {
		return errors.New("Err roleType")
	}
	bytes, err := permissions.Encode()
	return p.db.Put(([]byte)(PRE_NODE_PERMISSION+peerId), bytes)
}

// GetRoleType 获取peerId角色类型
func (p *nodePermission) GetRoleType(peerId string, blockHeight uint64) permission.RoleType {
	p.lock.Lock()
	defer p.lock.Unlock()
	var (
		info permission.MemberInfo
		ok   bool
	)

	if _, ok = p.supervisorMap[peerId]; ok {
		return permission.SUPERVISOR
	}
	permissionMap := p.permissionMap
	colleague := permissionMap.Colleague
	if info, ok = colleague[peerId]; ok {
		if info.Height == 0 || info.Height >= blockHeight {
			return permission.COLLEAGUE
		}
	}

	if info, ok = p.peerMap[peerId]; ok {
		if info.Height == 0 || info.Height >= blockHeight {
			return permission.PEER
		}
	}

	observer := permissionMap.Observer
	if info, ok = observer[peerId]; ok {
		if info.Height == 0 || info.Height >= blockHeight {
			return permission.OBSERVER
		}
	}
	return permission.OTHER
}

// CanInto 是否可进入
func (p *nodePermission) CanInto(peerId string, blockHeight uint64) bool {
	if p.chainAccessType == permission.STRICT {
		roleType := p.GetRoleType(peerId, blockHeight)
		if roleType >= permission.OTHER {
			p.p2pService.DropPeer(models.P2PID(peerId))
			p.log.Debug("The peer is no permission to into", "peer", peerId, "blockHeight", blockHeight)
			return false
		}
	}
	return true
}

// CanSync 是否可同步
func (p *nodePermission) CanSync(peerId string, blockHeight uint64) bool {
	if p.chainAccessType == permission.STRICT {
		roleType := p.GetRoleType(peerId, blockHeight)
		if roleType >= permission.OTHER {
			return false
		}
	}
	return true
}

// CanBroadcast 是否可同步
func (p *nodePermission) CanBroadcast(peerId string, blockHeight uint64) bool {
	if p.chainAccessType == permission.NONE {
		return true
	}

	roleType := p.GetRoleType(peerId, blockHeight)
	if roleType >= permission.OBSERVER {
		return false
	}
	return true
}

// CanPacker 是否可产块
func (p *nodePermission) CanPacker(peerId string, blockHeight uint64) bool {
	if p.chainAccessType == permission.NONE {
		return true
	}

	roleType := p.GetRoleType(peerId, blockHeight)
	if p.chainAccessType == permission.LIGHT {
		switch roleType {
		case permission.ADMIN:
			return true
		case permission.SUPERVISOR:
			return true
		case permission.COLLEAGUE:
			return true
		case permission.PEER:
			return true
		}
		return false
	}
	if p.chainAccessType == permission.STRICT {
		switch roleType {
		case permission.ADMIN:
			return true
		case permission.PEER:
			return true
		}
		return false
	}
	return true
}
