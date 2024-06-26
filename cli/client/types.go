/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2023-2023. All rights reserved.
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

package client

import "context"

// ResourceType Defines the resource type, e.g. secret, configmap...
type ResourceType string

// KubernetesClient Defines the capabilities that client needs to implement
type KubernetesClient interface {
	CLI() string
	GetNameSpace() (string, error)
	OperateResourceByYaml(yaml, operate string, ignoreNotfound bool) error
	DeleteResourceByQualifiedNames(qualifiedNames []string, namespace string) (string, error)
	DeleteFinalizersInResourceByQualifiedNames(qualifiedNames []string, namespace string) error
	GetResource(name []string, namespace, outputType string, resourceType ResourceType) ([]byte, error)
	CheckResourceExist(name, namespace string, resourceType ResourceType) (bool, error)

	GetObject(ctx context.Context, objectType ObjectType, namespace, nodeName string, outputType OutputType,
		data interface{}, objectName ...string) error
	ExecCmdInSpecifiedContainer(ctx context.Context, namespace, containerName, cmd string,
		podName ...string) ([]byte, error)
	CopyContainerFileToLocal(ctx context.Context, namespace, containerName, src, dst string,
		podName ...string) ([]byte, error)
	GetConsoleLogs(ctx context.Context, namespace, containerName string, isHistoryLogs bool,
		podName ...string) ([]byte, error)
}
