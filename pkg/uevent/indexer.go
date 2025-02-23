// This file is part of MinIO DirectPV
// Copyright (c) 2021, 2022 MinIO, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package uevent

import (
	"context"
	"errors"
	"time"

	directcsi "github.com/minio/directpv/pkg/apis/direct.csi.min.io/v1beta4"
	"github.com/minio/directpv/pkg/client"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

var (
	errNotDirectCSIDriveObject = errors.New("not a directcsidrive object")
)

type indexer struct {
	store  cache.Store
	nodeID string
}

func newIndexer(ctx context.Context, nodeID string, resyncPeriod time.Duration) *indexer {
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	lw := client.DrivesListerWatcher(nodeID)
	reflector := cache.NewReflector(lw, &directcsi.DirectCSIDrive{}, store, resyncPeriod)
	initResourceVersion := reflector.LastSyncResourceVersion()

	go reflector.Run(ctx.Done())

	if cache.WaitForCacheSync(
		ctx.Done(),
		func() bool {
			return reflector.LastSyncResourceVersion() != initResourceVersion
		},
	) {
		klog.Info("indexer successfully synced")
	} else {
		klog.Info("indexer can't be synced")
	}

	return &indexer{
		store:  store,
		nodeID: nodeID,
	}
}

func (i *indexer) filterDrivesByUEventFSUUID(fsuuid string) ([]*directcsi.DirectCSIDrive, error) {
	objects := i.store.List()
	filteredDrives := []*directcsi.DirectCSIDrive{}
	for _, obj := range objects {
		directCSIDrive, ok := obj.(*directcsi.DirectCSIDrive)
		if !ok {
			return nil, errNotDirectCSIDriveObject
		}
		if directCSIDrive.Status.NodeName != i.nodeID {
			continue
		}
		if directCSIDrive.Status.UeventFSUUID != fsuuid {
			continue
		}
		filteredDrives = append(filteredDrives, directCSIDrive)
	}
	return filteredDrives, nil
}

func (i *indexer) listDrives() (managedDrives, nonManagedDrives []*directcsi.DirectCSIDrive, err error) {
	objects := i.store.List()
	for _, obj := range objects {
		directCSIDrive, ok := obj.(*directcsi.DirectCSIDrive)
		if !ok {
			return nil, nil, errNotDirectCSIDriveObject
		}
		if directCSIDrive.Status.NodeName != i.nodeID {
			continue
		}
		if directCSIDrive.Status.DriveStatus == directcsi.DriveStatusInUse || directCSIDrive.Status.DriveStatus == directcsi.DriveStatusReady {
			managedDrives = append(managedDrives, directCSIDrive)
		} else {
			nonManagedDrives = append(nonManagedDrives, directCSIDrive)
		}
	}
	return managedDrives, nonManagedDrives, nil
}
