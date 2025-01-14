// +build integration

/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package integration

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"

	core "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"
	commonutil "k8s.io/minikube/pkg/util"
	"k8s.io/minikube/pkg/util/retry"
	"k8s.io/minikube/test/integration/util"
)

var (
	pvcName = "testpvc"
	pvcCmd  = []string{"get", "pvc", pvcName}
)

func testProvisioning(t *testing.T) {
	p := profileName(t)
	t.Parallel()

	kr := util.NewKubectlRunner(t, p)

	defer func() {
		if out, err := kr.RunCommand([]string{"delete", "pvc", pvcName}); err != nil {
			t.Logf("delete pvc %s failed: %v\noutput: %s\n", pvcName, err, out)
		}
	}()

	// We have to make sure the addon-manager has created the StorageClass before creating
	// a claim. Otherwise it will never get bound.

	checkStorageClass := func() error {
		scl := storage.StorageClassList{}
		if err := kr.RunCommandParseOutput([]string{"get", "storageclass"}, &scl); err != nil {
			return fmt.Errorf("get storageclass: %v", err)
		}

		if len(scl.Items) > 0 {
			return nil
		}
		return fmt.Errorf("no default StorageClass yet")
	}

	if err := retry.Expo(checkStorageClass, time.Second, 90*time.Second); err != nil {
		t.Fatalf("no default storage class after retry: %v", err)
	}

	// Check that the storage provisioner pod is running

	checkPodRunning := func() error {
		client, err := commonutil.GetClient(p)
		if err != nil {
			return errors.Wrap(err, "getting kubernetes client")
		}
		selector := labels.SelectorFromSet(labels.Set(map[string]string{"integration-test": "storage-provisioner"}))

		if err := commonutil.WaitForPodsWithLabelRunning(client, "kube-system", selector); err != nil {
			return err
		}
		return nil
	}

	if err := retry.Expo(checkPodRunning, 2*time.Second, 2*time.Minute); err != nil {
		t.Fatalf("Check storage-provisioner pod running failed with error: %v", err)
	}

	// Now create the PVC
	pvcPath := filepath.Join(*testdataDir, "pvc.yaml")
	if _, err := kr.RunCommand([]string{"create", "-f", pvcPath}); err != nil {
		t.Fatalf("Error creating pvc: %v", err)
	}

	// And check that it gets bound to a PV.
	checkStorage := func() error {
		pvc := core.PersistentVolumeClaim{}
		if err := kr.RunCommandParseOutput(pvcCmd, &pvc); err != nil {
			return err
		}
		// The test passes if the volume claim gets bound.
		if pvc.Status.Phase == "Bound" {
			return nil
		}
		return fmt.Errorf("PV not attached to PVC: %v", pvc)
	}

	if err := retry.Expo(checkStorage, 2*time.Second, 2*time.Minute); err != nil {
		t.Fatalf("PV Creation failed with error: %v", err)
	}

}
