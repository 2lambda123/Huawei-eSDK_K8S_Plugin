package volume

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"huawei-csi-driver/storage/oceanstor/client"
	"huawei-csi-driver/storage/oceanstor/smartx"
	"huawei-csi-driver/utils"
	"huawei-csi-driver/utils/log"
)

const (
	HYPERMETROPAIR_HEALTH_STATUS_FAULT    = "2"
	HYPERMETROPAIR_RUNNING_STATUS_NORMAL  = "1"
	HYPERMETROPAIR_RUNNING_STATUS_TO_SYNC = "100"
	HYPERMETROPAIR_RUNNING_STATUS_SYNCING = "23"
	HYPERMETROPAIR_RUNNING_STATUS_UNKNOWN = "0"
	HYPERMETROPAIR_RUNNING_STATUS_PAUSE   = "41"
	HYPERMETROPAIR_RUNNING_STATUS_ERROR   = "94"
	HYPERMETROPAIR_RUNNING_STATUS_INVALID = "35"

	HYPERMETRODOMAIN_RUNNING_STATUS_NORMAL = "1"
)

type Base struct {
	cli              *client.Client
	metroRemoteCli   *client.Client
	replicaRemoteCli *client.Client
	product          string
}

func (p *Base) commonPreCreate(ctx context.Context, params map[string]interface{}) error {
	analyzers := [...]func(context.Context, map[string]interface{}) error{
		p.getAllocType,
		p.getCloneSpeed,
		p.getPoolID,
		p.getQoS,
	}

	for _, analyzer := range analyzers {
		err := analyzer(ctx, params)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Base) getAllocType(_ context.Context, params map[string]interface{}) error {
	if v, exist := params["alloctype"].(string); exist && v == "thick" {
		params["alloctype"] = 0
	} else {
		params["alloctype"] = 1
	}

	return nil
}

func (p *Base) getCloneSpeed(_ context.Context, params map[string]interface{}) error {
	_, cloneExist := params["clonefrom"].(string)
	_, srcVolumeExist := params["sourcevolumename"].(string)
	_, srcSnapshotExist := params["sourcesnapshotname"].(string)
	if !(cloneExist || srcVolumeExist || srcSnapshotExist) {
		return nil
	}

	if v, exist := params["clonespeed"].(string); exist && v != "" {
		speed, err := strconv.Atoi(v)
		if err != nil || speed < 1 || speed > 4 {
			return fmt.Errorf("error config %s for clonespeed", v)
		}
		params["clonespeed"] = speed
	} else {
		params["clonespeed"] = 3
	}

	return nil
}

func (p *Base) getPoolID(ctx context.Context, params map[string]interface{}) error {
	poolName, exist := params["storagepool"].(string)
	if !exist || poolName == "" {
		return errors.New("must specify storage pool to create volume")
	}

	pool, err := p.cli.GetPoolByName(ctx, poolName)
	if err != nil {
		log.AddContext(ctx).Errorf("Get storage pool %s info error: %v", poolName, err)
		return err
	}
	if pool == nil {
		return fmt.Errorf("Storage pool %s doesn't exist", poolName)
	}

	params["poolID"] = pool["ID"].(string)

	return nil
}

func (p *Base) getQoS(ctx context.Context, params map[string]interface{}) error {
	if v, exist := params["qos"].(string); exist && v != "" {
		qos, err := smartx.ExtractQoSParameters(ctx, p.product, v)
		if err != nil {
			return utils.Errorf(ctx, "qos parameter %s error: %v", v, err)
		}

		validatedQos, err := smartx.ValidateQoSParameters(p.product, qos)
		if err != nil {
			return utils.Errorf(ctx, "validate qos parameters failed, error %v", err)
		}
		params["qos"] = validatedQos
	}

	return nil
}

func (p *Base) getRemotePoolID(ctx context.Context,
	params map[string]interface{}, remoteCli *client.Client) (string, error) {
	remotePool, exist := params["remotestoragepool"].(string)
	if !exist || len(remotePool) == 0 {
		msg := "no remote pool is specified"
		log.AddContext(ctx).Errorln(msg)
		return "", errors.New(msg)
	}

	pool, err := remoteCli.GetPoolByName(ctx, remotePool)
	if err != nil {
		log.AddContext(ctx).Errorf("Get remote storage pool %s info error: %v", remotePool, err)
		return "", err
	}
	if pool == nil {
		return "", fmt.Errorf("remote storage pool %s doesn't exist", remotePool)
	}

	return pool["ID"].(string), nil
}

func (p *Base) preExpandCheckCapacity(ctx context.Context,
	params, taskResult map[string]interface{}) (map[string]interface{}, error) {
	// check the local pool
	localParentName := params["localParentName"].(string)
	pool, err := p.cli.GetPoolByName(ctx, localParentName)
	if err != nil || pool == nil {
		msg := fmt.Sprintf("Get storage pool %s info error: %v", localParentName, err)
		log.AddContext(ctx).Errorf(msg)
		return nil, errors.New(msg)
	}

	return nil, nil
}

func (p *Base) getSnapshotReturnInfo(snapshot map[string]interface{}, snapshotSize int64) map[string]interface{} {
	snapshotCreated, _ := strconv.ParseInt(snapshot["TIMESTAMP"].(string), 10, 64)
	snapshotSizeBytes := snapshotSize * 512
	return map[string]interface{}{
		"CreationTime": snapshotCreated,
		"SizeBytes":    snapshotSizeBytes,
		"ParentID":     snapshot["PARENTID"].(string),
	}
}

func (p *Base) createReplicationPair(ctx context.Context,
	params, taskResult map[string]interface{}) (map[string]interface{}, error) {
	resType := taskResult["resType"].(int)
	remoteDeviceID := taskResult["remoteDeviceID"].(string)

	var localID string
	var remoteID string

	if resType == 11 {
		localID = taskResult["localLunID"].(string)
		remoteID = taskResult["remoteLunID"].(string)
	} else {
		localID = taskResult["localFSID"].(string)
		remoteID = taskResult["remoteFSID"].(string)
	}

	data := map[string]interface{}{
		"LOCALRESID":       localID,
		"LOCALRESTYPE":     resType,
		"REMOTEDEVICEID":   remoteDeviceID,
		"REMOTERESID":      remoteID,
		"REPLICATIONMODEL": 2, // asynchronous replication
		"SYNCHRONIZETYPE":  2, // timed wait after synchronization begins
		"SPEED":            4, // highest speed
	}

	replicationSyncPeriod, exist := params["replicationSyncPeriod"]
	if exist {
		data["TIMINGVAL"] = replicationSyncPeriod
	}

	vStorePairID, exist := taskResult["vStorePairID"]
	if exist {
		data["VSTOREPAIRID"] = vStorePairID
	}

	pair, err := p.cli.CreateReplicationPair(ctx, data)
	if err != nil {
		log.AddContext(ctx).Errorf("Create replication pair error: %v", err)
		return nil, err
	}

	pairID := pair["ID"].(string)
	err = p.cli.SyncReplicationPair(ctx, pairID)
	if err != nil {
		log.AddContext(ctx).Errorf("Sync replication pair %s error: %v", pairID, err)
		p.cli.DeleteReplicationPair(ctx, pairID)
		return nil, err
	}

	return nil, nil
}

func (p *Base) getRemoteDeviceID(ctx context.Context, deviceSN string) (string, error) {
	remoteDevice, err := p.cli.GetRemoteDeviceBySN(ctx, deviceSN)
	if err != nil {
		log.AddContext(ctx).Errorf("Get remote device %s error: %v", deviceSN, err)
		return "", err
	}
	if remoteDevice == nil {
		msg := fmt.Sprintf("Remote device of SN %s does not exist", deviceSN)
		log.AddContext(ctx).Errorln(msg)
		return "", errors.New(msg)
	}

	if remoteDevice["HEALTHSTATUS"] != REMOTE_DEVICE_HEALTH_STATUS ||
		remoteDevice["RUNNINGSTATUS"] != REMOTE_DEVICE_RUNNING_STATUS_LINK_UP {
		msg := fmt.Sprintf("Remote device %s status is not normal", deviceSN)
		log.AddContext(ctx).Errorln(msg)
		return "", errors.New(msg)
	}

	return remoteDevice["ID"].(string), nil
}

func (p *Base) getWorkLoadIDByName(ctx context.Context,
	cli *client.Client,
	workloadTypeName string) (string, error) {
	workloadTypeID, err := cli.GetApplicationTypeByName(ctx, workloadTypeName)
	if err != nil {
		log.AddContext(ctx).Errorf("Get application types returned error: %v", err)
		return "", err
	}
	if workloadTypeID == "" {
		msg := fmt.Sprintf("The workloadType %s does not exist on storage", workloadTypeName)
		log.AddContext(ctx).Errorln(msg)
		return "", errors.New(msg)
	}
	return workloadTypeID, nil
}

func (p *Base) setWorkLoadID(ctx context.Context, cli *client.Client, params map[string]interface{}) error {
	if val, ok := params["applicationtype"].(string); ok {
		workloadTypeID, err := p.getWorkLoadIDByName(ctx, cli, val)
		if err != nil {
			return err
		}
		params["workloadTypeID"] = workloadTypeID
	}
	return nil
}

func (p *Base) prepareVolObj(ctx context.Context, params, res map[string]interface{}) utils.Volume {
	volName, isStr := params["name"].(string)
	if !isStr {
		// Not expecting this error to happen
		log.AddContext(ctx).Warningf("Expecting string for volume name, received type %T", params["name"])
	}
	volObj := utils.NewVolume(volName)
	if res != nil {
		if lunWWN, ok := res["lunWWN"].(string); ok {
			volObj.SetLunWWN(lunWWN)
		}
	}
	return volObj
}