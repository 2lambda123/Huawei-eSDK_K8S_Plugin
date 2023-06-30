/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2020-2023. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"huawei-csi-driver/proto"
	"huawei-csi-driver/storage/oceanstor/attacher"
	"huawei-csi-driver/storage/oceanstor/client"
	"huawei-csi-driver/storage/oceanstor/volume"
	"huawei-csi-driver/utils"
	"huawei-csi-driver/utils/log"
)

const (
	hyperMetroPairRunningStatusNormal = "1"
	hyperMetroPairRunningStatusPause  = "41"
	reflectResultLength               = 2
)

type OceanstorSanPlugin struct {
	OceanstorPlugin
	protocol string
	portals  []string
	alua     map[string]interface{}

	replicaRemotePlugin *OceanstorSanPlugin
	metroRemotePlugin   *OceanstorSanPlugin
	storageOnline       bool
	clientCount         int
	clientMutex         sync.Mutex
}

type handlerRequest struct {
	localCli   client.BaseClientInterface
	metroCli   client.BaseClientInterface
	lun        map[string]interface{}
	parameters map[string]interface{}
	method     string
}

func init() {
	RegPlugin("oceanstor-san", &OceanstorSanPlugin{})
}

func (p *OceanstorSanPlugin) NewPlugin() Plugin {
	return &OceanstorSanPlugin{}
}

func (p *OceanstorSanPlugin) Init(config, parameters map[string]interface{}, keepLogin bool) error {
	protocol, exist := parameters["protocol"].(string)
	if !exist || (protocol != "iscsi" && protocol != "fc" && protocol != "roce" && protocol != "fc-nvme") {
		return errors.New("protocol must be provided as 'iscsi', 'fc', " +
			"'roce' or 'fc-nvme' for oceanstor-san backend")
	}

	p.alua, _ = parameters["ALUA"].(map[string]interface{})

	if protocol == "iscsi" || protocol == "roce" {
		portals, exist := parameters["portals"].([]interface{})
		if !exist {
			return errors.New("portals are required to configure for iSCSI or RoCE backend")
		}

		IPs, err := proto.VerifyIscsiPortals(portals)
		if err != nil {
			return err
		}

		p.portals = IPs
	}

	err := p.init(config, keepLogin)
	if err != nil {
		return err
	}

	if (protocol == "roce" || protocol == "fc-nvme") && p.product != "DoradoV6" {
		msg := fmt.Sprintf("The storage backend %s does not support NVME protocol", p.product)
		log.Errorln(msg)
		return errors.New(msg)
	}

	p.protocol = protocol
	p.storageOnline = true

	return nil
}

func (p *OceanstorSanPlugin) getSanObj() *volume.SAN {
	var metroRemoteCli client.BaseClientInterface
	var replicaRemoteCli client.BaseClientInterface

	if p.metroRemotePlugin != nil {
		metroRemoteCli = p.metroRemotePlugin.cli
	}
	if p.replicaRemotePlugin != nil {
		replicaRemoteCli = p.replicaRemotePlugin.cli
	}

	return volume.NewSAN(p.cli, metroRemoteCli, replicaRemoteCli, p.product)
}

func (p *OceanstorSanPlugin) CreateVolume(ctx context.Context,
	name string,
	parameters map[string]interface{}) (utils.Volume, error) {
	size, ok := parameters["size"].(int64)
	if !ok || !utils.IsCapacityAvailable(size, SectorSize) {
		msg := fmt.Sprintf("Create Volume: the capacity %d is not an integer multiple of 512.", size)
		log.AddContext(ctx).Errorln(msg)
		return nil, errors.New(msg)
	}

	params := p.getParams(ctx, name, parameters)
	san := p.getSanObj()

	volObl, err := san.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	return volObl, nil
}

func (p *OceanstorSanPlugin) QueryVolume(ctx context.Context, name string, params map[string]interface{}) (
	utils.Volume, error) {
	san := p.getSanObj()
	return san.Query(ctx, name)
}

func (p *OceanstorSanPlugin) DeleteVolume(ctx context.Context, name string) error {
	san := p.getSanObj()
	return san.Delete(ctx, name)
}

func (p *OceanstorSanPlugin) ExpandVolume(ctx context.Context, name string, size int64) (bool, error) {
	if !utils.IsCapacityAvailable(size, SectorSize) {
		msg := fmt.Sprintf("Expand Volume: the capacity %d is not an integer multiple of 512.", size)
		log.AddContext(ctx).Errorln(msg)
		return false, errors.New(msg)
	}
	san := p.getSanObj()
	newSize := utils.TransVolumeCapacity(size, 512)
	isAttach, err := san.Expand(ctx, name, newSize)
	return isAttach, err
}

func (p *OceanstorSanPlugin) isHyperMetro(ctx context.Context, lun map[string]interface{}) bool {
	rssStr, ok := lun["HASRSSOBJECT"].(string)
	if !ok {
		log.AddContext(ctx).Errorf("get lun HASRSSOBJECT failed, lun[\"HASRSSOBJECT\"]:%v", lun["HASRSSOBJECT"])
		return false
	}

	var rss map[string]string
	if err := json.Unmarshal([]byte(rssStr), &rss); err != nil {
		log.AddContext(ctx).Errorf("unmarshal lun HASRSSOBJECT failed, lun[\"HASRSSOBJECT\"]:%s", rssStr)
		return false
	}

	if hyperMetro, ok := rss["HyperMetro"]; ok && hyperMetro == "TRUE" {
		return true
	}
	return false
}

func (p *OceanstorSanPlugin) metroHandler(ctx context.Context, req handlerRequest) ([]reflect.Value, error) {
	localLunID := req.lun["ID"].(string)
	pair, err := req.localCli.GetHyperMetroPairByLocalObjID(ctx, localLunID)
	if err != nil {
		return nil, err
	}
	if pair == nil {
		return nil, fmt.Errorf("hypermetro pair of LUN %s doesn't exist", localLunID)
	}

	if req.method == "ControllerDetach" || req.method == "NodeUnstage" {
		if pair["RUNNINGSTATUS"] != hyperMetroPairRunningStatusNormal &&
			pair["RUNNINGSTATUS"] != hyperMetroPairRunningStatusPause {
			log.AddContext(ctx).Warningf("hypermetro pair status of LUN %s is not normal or pause",
				localLunID)
		}
	} else {
		if pair["RUNNINGSTATUS"] != hyperMetroPairRunningStatusNormal {
			log.AddContext(ctx).Warningf("hypermetro pair status of LUN %s is not normal", localLunID)
		}
	}

	localAttacher := attacher.NewAttacher(p.product, req.localCli, p.protocol, "csi", p.portals, p.alua)
	remoteAttacher := attacher.NewAttacher(p.metroRemotePlugin.product, req.metroCli, p.metroRemotePlugin.protocol,
		"csi", p.metroRemotePlugin.portals, p.metroRemotePlugin.alua)

	metroAttacher := attacher.NewMetroAttacher(localAttacher, remoteAttacher, p.protocol)
	lunName := req.lun["NAME"].(string)
	out := utils.ReflectCall(metroAttacher, req.method, ctx, lunName, req.parameters)

	return out, nil
}

func (p *OceanstorSanPlugin) commonHandler(ctx context.Context,
	plugin *OceanstorSanPlugin, lun, parameters map[string]interface{},
	method string) ([]reflect.Value, error) {
	commonAttacher := attacher.NewAttacher(plugin.product, plugin.cli, plugin.protocol, "csi",
		plugin.portals, plugin.alua)

	lunName, ok := lun["NAME"].(string)
	if !ok {
		return nil, errors.New("there is no NAME in lun info")
	}
	out := utils.ReflectCall(commonAttacher, method, ctx, lunName, parameters)
	return out, nil
}

func (p *OceanstorSanPlugin) handler(ctx context.Context, req handlerRequest) ([]reflect.Value, error) {
	var out []reflect.Value
	var err error

	if !p.isHyperMetro(ctx, req.lun) {
		return p.commonHandler(ctx, p, req.lun, req.parameters, req.method)
	}

	if p.storageOnline && p.metroRemotePlugin != nil && p.metroRemotePlugin.storageOnline {
		out, err = p.metroHandler(ctx, req)
	} else if p.storageOnline {
		log.AddContext(ctx).Warningf("the lun %s is hyperMetro, but just the local storage is online",
			req.lun["NAME"].(string))
		out, err = p.commonHandler(ctx, p, req.lun, req.parameters, req.method)
	} else if p.metroRemotePlugin != nil && p.metroRemotePlugin.storageOnline {
		log.AddContext(ctx).Warningf("the lun %s is hyperMetro, but just the remote storage is online",
			req.lun["NAME"].(string))
		out, err = p.commonHandler(ctx, p.metroRemotePlugin, req.lun, req.parameters, req.method)
	}

	return out, err
}

// AttachVolume attach volume to node,return storage mapping info.
func (p *OceanstorSanPlugin) AttachVolume(ctx context.Context, name string,
	parameters map[string]interface{}) (map[string]interface{}, error) {
	var localCli, metroCli client.BaseClientInterface
	if p.storageOnline {
		localCli = p.cli
	}

	if p.metroRemotePlugin != nil && p.metroRemotePlugin.storageOnline {
		metroCli = p.metroRemotePlugin.cli
	}

	lunName := p.cli.MakeLunName(name)
	lun, err := p.getLunInfo(ctx, localCli, metroCli, lunName)
	if err != nil {
		log.AddContext(ctx).Errorf("Get lun %s error: %v", lunName, err)
		return nil, err
	}
	if lun == nil {
		return nil, utils.Errorf(ctx, "Get empty lun info, lunName: %v", lunName)
	}

	var out []reflect.Value
	out, err = p.handler(ctx, handlerRequest{localCli: localCli, metroCli: metroCli,
		lun: lun, parameters: parameters, method: "ControllerAttach"})
	if err != nil {
		return nil, utils.Errorf(ctx, "Storage connect for volume %s error: %v", lunName, err)
	}

	if len(out) != reflectResultLength {
		return nil, utils.Errorf(ctx, "attach volume %s error", lunName)
	}

	result := out[1].Interface()
	if result != nil {
		return nil, result.(error)
	}

	connectInfo, ok := out[0].Interface().(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("controller attach volume %s error", lunName)
	}
	return connectInfo, nil
}

func (p *OceanstorSanPlugin) DetachVolume(ctx context.Context, name string, parameters map[string]interface{}) error {
	var localCli, metroCli client.BaseClientInterface
	if p.storageOnline {
		localCli = p.cli
	}

	if p.metroRemotePlugin != nil && p.metroRemotePlugin.storageOnline {
		metroCli = p.metroRemotePlugin.cli
	}

	lunName := p.cli.MakeLunName(name)
	lun, err := p.getLunInfo(ctx, localCli, metroCli, lunName)
	if err != nil {
		log.AddContext(ctx).Errorf("Get lun %s error: %v", lunName, err)
		return err
	}
	if lun == nil {
		log.AddContext(ctx).Warningf("LUN %s to detach doesn't exist", lunName)
		return nil
	}

	var out []reflect.Value
	out, err = p.handler(ctx, handlerRequest{localCli: localCli, metroCli: metroCli,
		lun: lun, parameters: parameters, method: "ControllerDetach"})
	if err != nil {
		return err
	}
	if len(out) != reflectResultLength {
		return fmt.Errorf("detach volume %s error", lunName)
	}

	result := out[1].Interface()
	if result != nil {
		return result.(error)
	}

	return nil
}

func (p *OceanstorSanPlugin) mutexReleaseClient(ctx context.Context,
	plugin *OceanstorSanPlugin,
	cli client.BaseClientInterface) {
	plugin.clientMutex.Lock()
	defer plugin.clientMutex.Unlock()
	plugin.clientCount--
	if plugin.clientCount == 0 {
		cli.Logout(ctx)
		plugin.storageOnline = false
	}
}

func (p *OceanstorSanPlugin) releaseClient(ctx context.Context, cli, metroCli client.BaseClientInterface) {
	if p.storageOnline {
		p.mutexReleaseClient(ctx, p, cli)
	}

	if p.metroRemotePlugin != nil && p.metroRemotePlugin.storageOnline {
		p.mutexReleaseClient(ctx, p.metroRemotePlugin, metroCli)
	}
}

func (p *OceanstorSanPlugin) UpdatePoolCapabilities(poolNames []string) (map[string]interface{}, error) {
	return p.updatePoolCapabilities(poolNames, "1")
}

func (p *OceanstorSanPlugin) UpdateReplicaRemotePlugin(remote Plugin) {
	p.replicaRemotePlugin = remote.(*OceanstorSanPlugin)
}

func (p *OceanstorSanPlugin) UpdateMetroRemotePlugin(remote Plugin) {
	p.metroRemotePlugin = remote.(*OceanstorSanPlugin)
}

func (p *OceanstorSanPlugin) CreateSnapshot(ctx context.Context,
	lunName, snapshotName string) (map[string]interface{}, error) {
	san := p.getSanObj()

	snapshotName = utils.GetSnapshotName(snapshotName)
	snapshot, err := san.CreateSnapshot(ctx, lunName, snapshotName)
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (p *OceanstorSanPlugin) DeleteSnapshot(ctx context.Context,
	snapshotParentID, snapshotName string) error {
	san := p.getSanObj()

	snapshotName = utils.GetSnapshotName(snapshotName)
	err := san.DeleteSnapshot(ctx, snapshotName)
	if err != nil {
		return err
	}

	return nil
}

func (p *OceanstorSanPlugin) mutexGetClient(ctx context.Context) (client.BaseClientInterface, error) {
	p.clientMutex.Lock()
	defer p.clientMutex.Unlock()
	var err error
	if !p.storageOnline || p.clientCount == 0 {
		err = p.cli.Login(ctx)
		p.storageOnline = err == nil
		if err == nil {
			p.clientCount++
		}
	} else {
		p.clientCount++
	}

	return p.cli, err
}

func (p *OceanstorSanPlugin) getClient(ctx context.Context) (client.BaseClientInterface, client.BaseClientInterface, error) {
	cli, locErr := p.mutexGetClient(ctx)
	var metroCli client.BaseClientInterface
	var rmtErr error
	if p.metroRemotePlugin != nil {
		metroCli, rmtErr = p.metroRemotePlugin.mutexGetClient(ctx)
		if locErr != nil && rmtErr != nil {
			return nil, nil, errors.New("local and remote storage can not login")
		}
	} else {
		if locErr != nil {
			return nil, nil, errors.New("local storage can not login")
		}
	}
	return cli, metroCli, nil
}

func (p *OceanstorSanPlugin) getLunInfo(ctx context.Context, localCli, remoteCli client.BaseClientInterface,
	lunName string) (map[string]interface{}, error) {
	var lun map[string]interface{}
	var err error
	if p.storageOnline {
		lun, err = localCli.GetLunByName(ctx, lunName)
	} else if p.metroRemotePlugin != nil && p.metroRemotePlugin.storageOnline {
		lun, err = remoteCli.GetLunByName(ctx, lunName)
	} else {
		return nil, errors.New("both the local and remote storage are not online")
	}

	return lun, err
}

// UpdateBackendCapabilities to update the block storage capabilities
func (p *OceanstorSanPlugin) UpdateBackendCapabilities() (map[string]interface{}, map[string]interface{}, error) {
	capabilities, specifications, err := p.OceanstorPlugin.UpdateBackendCapabilities()
	if err != nil {
		p.storageOnline = false
		return nil, nil, err
	}

	p.storageOnline = true
	p.updateHyperMetroCapability(capabilities)
	p.updateReplicaCapability(capabilities)
	return capabilities, specifications, nil
}

func (p *OceanstorSanPlugin) updateHyperMetroCapability(capabilities map[string]interface{}) {
	if metroSupport, exist := capabilities["SupportMetro"]; !exist || metroSupport == false {
		return
	}

	capabilities["SupportMetro"] = p.metroRemotePlugin != nil &&
		p.storageOnline && p.metroRemotePlugin.storageOnline
}

func (p *OceanstorSanPlugin) updateReplicaCapability(capabilities map[string]interface{}) {
	if metroReplica, exist := capabilities["SupportReplication"]; !exist || metroReplica == false {
		return
	}

	capabilities["SupportReplication"] = p.replicaRemotePlugin != nil
}

func (p *OceanstorSanPlugin) Validate(ctx context.Context, param map[string]interface{}) error {
	log.AddContext(ctx).Infoln("Start to validate OceanstorSanPlugin parameters.")

	err := p.verifyOceanstorSanParam(ctx, param)
	if err != nil {
		return err
	}

	clientConfig, err := p.getNewClientConfig(ctx, param)
	if err != nil {
		return err
	}

	// Login verification
	cli := client.NewClient(clientConfig)
	err = cli.ValidateLogin(ctx)
	if err != nil {
		return err
	}
	cli.Logout(ctx)

	return nil
}

func (p *OceanstorSanPlugin) verifyOceanstorSanParam(ctx context.Context, config map[string]interface{}) error {
	parameters, exist := config["parameters"].(map[string]interface{})
	if !exist {
		msg := fmt.Sprintf("Verify parameters: [%v] failed. \nparameters must be provided", config["parameters"])
		log.AddContext(ctx).Errorln(msg)
		return errors.New(msg)
	}

	protocol, exist := parameters["protocol"].(string)
	if !exist || (protocol != "iscsi" && protocol != "fc" && protocol != "roce" && protocol != "fc-nvme") {
		msg := fmt.Sprintf("Verify protocol: [%v] failed. \nprotocol must be provided and be one of "+
			"[iscsi, fc, roce, fc-nvme] for oceanstor-san backend\n", parameters["protocol"])
		log.AddContext(ctx).Errorln(msg)
		return errors.New(msg)
	}

	if protocol == "iscsi" || protocol == "roce" {
		portals, exist := parameters["portals"].([]interface{})
		if !exist {
			msg := fmt.Sprintf("Verify portals: [%v] failed. \nportals are required to configure for "+
				"iscsi or roce for oceanstor-san backend\n", parameters["portals"])
			log.AddContext(ctx).Errorln(msg)
			return errors.New(msg)
		}

		_, err := proto.VerifyIscsiPortals(portals)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *OceanstorSanPlugin) DeleteDTreeVolume(ctx context.Context, m map[string]interface{}) error {
	return errors.New("not implement")
}

func (p *OceanstorSanPlugin) ExpandDTreeVolume(ctx context.Context, m map[string]interface{}) (bool, error) {
	return false, errors.New("not implement")
}
