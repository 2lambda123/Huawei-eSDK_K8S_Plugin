/*
 Copyright (c) Huawei Technologies Co., Ltd. 2022-2023. All rights reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
      http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	xuanwuv1 "huawei-csi-driver/client/apis/xuanwu/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeStorageBackendClaims implements StorageBackendClaimInterface
type FakeStorageBackendClaims struct {
	Fake *FakeXuanwuV1
	ns   string
}

var storagebackendclaimsResource = schema.GroupVersionResource{Group: "xuanwu.huawei.io", Version: "v1", Resource: "storagebackendclaims"}

var storagebackendclaimsKind = schema.GroupVersionKind{Group: "xuanwu.huawei.io", Version: "v1", Kind: "StorageBackendClaim"}

// Get takes name of the storageBackendClaim, and returns the corresponding storageBackendClaim object, and an error if there is any.
func (c *FakeStorageBackendClaims) Get(ctx context.Context, name string, options v1.GetOptions) (result *xuanwuv1.StorageBackendClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(storagebackendclaimsResource, c.ns, name), &xuanwuv1.StorageBackendClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*xuanwuv1.StorageBackendClaim), err
}

// List takes label and field selectors, and returns the list of StorageBackendClaims that match those selectors.
func (c *FakeStorageBackendClaims) List(ctx context.Context, opts v1.ListOptions) (result *xuanwuv1.StorageBackendClaimList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(storagebackendclaimsResource, storagebackendclaimsKind, c.ns, opts), &xuanwuv1.StorageBackendClaimList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &xuanwuv1.StorageBackendClaimList{ListMeta: obj.(*xuanwuv1.StorageBackendClaimList).ListMeta}
	for _, item := range obj.(*xuanwuv1.StorageBackendClaimList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested storageBackendClaims.
func (c *FakeStorageBackendClaims) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(storagebackendclaimsResource, c.ns, opts))

}

// Create takes the representation of a storageBackendClaim and creates it.  Returns the server's representation of the storageBackendClaim, and an error, if there is any.
func (c *FakeStorageBackendClaims) Create(ctx context.Context, storageBackendClaim *xuanwuv1.StorageBackendClaim, opts v1.CreateOptions) (result *xuanwuv1.StorageBackendClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(storagebackendclaimsResource, c.ns, storageBackendClaim), &xuanwuv1.StorageBackendClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*xuanwuv1.StorageBackendClaim), err
}

// Update takes the representation of a storageBackendClaim and updates it. Returns the server's representation of the storageBackendClaim, and an error, if there is any.
func (c *FakeStorageBackendClaims) Update(ctx context.Context, storageBackendClaim *xuanwuv1.StorageBackendClaim, opts v1.UpdateOptions) (result *xuanwuv1.StorageBackendClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(storagebackendclaimsResource, c.ns, storageBackendClaim), &xuanwuv1.StorageBackendClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*xuanwuv1.StorageBackendClaim), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeStorageBackendClaims) UpdateStatus(ctx context.Context, storageBackendClaim *xuanwuv1.StorageBackendClaim, opts v1.UpdateOptions) (*xuanwuv1.StorageBackendClaim, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(storagebackendclaimsResource, "status", c.ns, storageBackendClaim), &xuanwuv1.StorageBackendClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*xuanwuv1.StorageBackendClaim), err
}

// Delete takes name of the storageBackendClaim and deletes it. Returns an error if one occurs.
func (c *FakeStorageBackendClaims) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(storagebackendclaimsResource, c.ns, name), &xuanwuv1.StorageBackendClaim{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeStorageBackendClaims) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(storagebackendclaimsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &xuanwuv1.StorageBackendClaimList{})
	return err
}

// Patch applies the patch and returns the patched storageBackendClaim.
func (c *FakeStorageBackendClaims) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *xuanwuv1.StorageBackendClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(storagebackendclaimsResource, c.ns, name, pt, data, subresources...), &xuanwuv1.StorageBackendClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*xuanwuv1.StorageBackendClaim), err
}
