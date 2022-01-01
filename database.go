// Package permission
//
// @author: xwc1125
package permission

import (
	"github.com/chain5j/chain5j-pkg/codec/rlp"
	"github.com/chain5j/chain5j-pkg/database/kvstore"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/chain5j-protocol/models/permission"
)

const (
	PRE_TYPE            = "permission_type"       // 权限类型
	PRE_ADMIN           = "permission_admin"      // 管理员
	PRE_SUPERVISOR      = "permission_supervisor" // 监管
	PRE_PEER            = "permission_peer"       // 成员
	PRE_NODE_PERMISSION = "permission_node"       // 节点
)

// writePermissionAlloc 写入权限
func writePermissionAlloc(db kvstore.Database, permission permission.GenesisPermission) error {
	// write supervisor
	var (
		err   error
		bytes []byte
	)
	batch := db.NewBatch()

	// 写入权限类型
	{
		bytes, err = rlp.EncodeToBytes(&permission.Type)
		if err != nil {
			return err
		}
		err = batch.Put(([]byte)(PRE_TYPE), bytes)
		if err != nil {
			return err
		}
	}

	// 写入监管信息
	{
		if permission.SupervisorAlloc != nil && len(permission.SupervisorAlloc) > 0 {
			bytes, err = rlp.EncodeToBytes(&permission.SupervisorAlloc)
			if err != nil {
				return err
			}
			err = batch.Put(([]byte)(PRE_SUPERVISOR), bytes)
			if err != nil {
				return err
			}
		}
	}

	// 写入每个节点的权限
	{
		for k, v := range permission.PeerPermissionAlloc {
			bytes, err = v.Encode()
			if err != nil {
				return err
			}
			err = batch.Put(([]byte)(PRE_NODE_PERMISSION+k), bytes)
		}
		if err != nil {
			return err
		}
	}

	return batch.Write()
}

// writeMemberInfoMap 根据主键写入其配置信息
func writeMemberInfoMap(db kvstore.Database, dbKey string, infoMap permission.MemberInfoMap) error {
	bytes, err := rlp.EncodeToBytes(&infoMap)
	if err != nil {
		return err
	}
	err = db.Put(([]byte)(dbKey), bytes)
	if err != nil {
		return err
	}
	return nil
}

// getChainAccessType 从数据库中获取权限类型
func getChainAccessType(db kvstore.Database) (permission.ChainAccessType, error) {
	bytes, err := db.Get(([]byte)(PRE_TYPE))
	if err != nil {
		if err.Error() == errNoFount {
			return permission.STRICT, nil
		}
		return permission.STRICT, err
	}
	var permissionType permission.ChainAccessType
	err = rlp.DecodeBytes(bytes, &permissionType)
	return permissionType, err
}

// getAllAdmins 从数据库中读取所有的admin
func getAllAdmins(db kvstore.Database) (permission.MemberInfoMap, error) {
	memberInfoMap := permission.NewMemberInfoMap()
	bytes, err := db.Get(([]byte)(PRE_ADMIN))
	if err != nil {
		if err.Error() == errNoFount {
			return memberInfoMap, nil
		}
		return nil, err
	}
	err = rlp.DecodeBytes(bytes, &memberInfoMap)
	return memberInfoMap, err
}

// getAllSupervisors 从数据库中读取所有的Supervisors
func getAllSupervisors(db kvstore.Database) (permission.MemberInfoMap, error) {
	supervisorInfos := permission.NewMemberInfoMap()
	bytes, err := db.Get(([]byte)(PRE_SUPERVISOR))
	if err != nil {
		if err.Error() == errNoFount {
			return supervisorInfos, nil
		}
		return nil, err
	}
	err = rlp.DecodeBytes(bytes, &supervisorInfos)
	return supervisorInfos, err
}

// getAllPeers 从数据库中读取所有的共识成员
func getAllPeers(db kvstore.Database) (permission.MemberInfoMap, error) {
	memberInfoMap := permission.NewMemberInfoMap()
	bytes, err := db.Get(([]byte)(PRE_PEER))
	if err != nil {
		if err.Error() == errNoFount {
			return memberInfoMap, nil
		}
		return nil, err
	}
	err = rlp.DecodeBytes(bytes, &memberInfoMap)
	return memberInfoMap, err
}

// getPeerPermissions 从数据库中读取peerId对应的权限
func getPeerPermissions(db kvstore.Database, peerId models.P2PID) (permission.PeerPermissionAlloc, error) {
	var nodePermissionAlloc permission.PeerPermissionAlloc
	bytes, err := db.Get(([]byte)(PRE_NODE_PERMISSION + peerId))
	if err != nil {
		if err.Error() == errNoFount {
			return nodePermissionAlloc, nil
		}
		return nodePermissionAlloc, err
	}
	err = nodePermissionAlloc.Decode(bytes)
	if err != nil {
		return nodePermissionAlloc, err
	}
	return nodePermissionAlloc, nil
}
