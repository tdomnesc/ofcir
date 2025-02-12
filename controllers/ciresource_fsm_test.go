package controllers

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/providers"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestCIResourceFSMProcess(t *testing.T) {

	now := v1.Now()

	fakePool := &ofcirv1.CIPool{
		ObjectMeta: v1.ObjectMeta{
			Name: "fake-pool",
		},
		Spec: ofcirv1.CIPoolSpec{
			Provider: string(providers.ProviderDummy),
		},
	}

	tests := []struct {
		name                    string
		cir                     *ofcirv1.CIResource
		cipool                  *ofcirv1.CIPool
		expectedIsResourceDirty bool
		expectedIsStatusDirty   bool
		expectedState           ofcirv1.CIResourceState
		expectedRetryAfter      time.Duration
		expectedError           bool
	}{
		{
			name: "init->init (no finalizer)",
			cir: &ofcirv1.CIResource{
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateNone,
				},
			},
			cipool: fakePool,

			expectedIsResourceDirty: true,
			expectedState:           ofcirv1.StateNone,
			expectedRetryAfter:      defaultCirRetryDelay,
		},
		{
			name: "init->provisioning (with finalizer)",
			cir: &ofcirv1.CIResource{
				ObjectMeta: v1.ObjectMeta{
					Finalizers: []string{
						ofcirv1.CIResourceFinalizer,
					},
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateNone,
				},
			},
			cipool: fakePool,

			expectedIsStatusDirty: true,
			expectedState:         ofcirv1.StateProvisioning,
			expectedRetryAfter:    defaultCirRetryDelay,
		},
		{
			name: "provisioning->provisioning-wait (acquired)",
			cir: &ofcirv1.CIResource{
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateProvisioning,
				},
			},
			cipool: fakePool,

			expectedIsStatusDirty: true,
			expectedState:         ofcirv1.StateProvisioningWait,
			expectedRetryAfter:    defaultCirRetryDelay,
		},
		{
			name: "provisioning-wait=>available (provisioned)",
			cir: &ofcirv1.CIResource{
				Status: ofcirv1.CIResourceStatus{
					ResourceId: "dummy-0",
					State:      ofcirv1.StateProvisioningWait,
				},
			},
			cipool: fakePool,

			expectedIsStatusDirty: true,
			expectedState:         ofcirv1.StateAvailable,
			expectedRetryAfter:    0,
		},
		{
			name: "available->maintenance",
			cir: &ofcirv1.CIResource{
				Spec: ofcirv1.CIResourceSpec{
					State: ofcirv1.StateMaintenance,
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateAvailable,
				},
			},
			cipool:                fakePool,
			expectedIsStatusDirty: true,
			expectedState:         ofcirv1.StateMaintenance,
			expectedRetryAfter:    defaultCirRetryDelay,
		},
		{
			name: "available->inuse",
			cir: &ofcirv1.CIResource{
				Spec: ofcirv1.CIResourceSpec{
					State: ofcirv1.StateInUse,
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateAvailable,
				},
			},
			cipool:                fakePool,
			expectedIsStatusDirty: true,
			expectedState:         ofcirv1.StateInUse,
			expectedRetryAfter:    defaultCirRetryDelay,
		},
		{
			name: "available->delete",
			cir: &ofcirv1.CIResource{
				ObjectMeta: v1.ObjectMeta{
					DeletionTimestamp: &now,
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateAvailable,
				},
			},
			cipool:                fakePool,
			expectedIsStatusDirty: true,
			expectedState:         ofcirv1.StateDelete,
			expectedRetryAfter:    defaultCirRetryDelay,
		},
		{
			name: "maintenance->available",
			cir: &ofcirv1.CIResource{
				Spec: ofcirv1.CIResourceSpec{
					State: ofcirv1.StateAvailable,
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateMaintenance,
				},
			},
			cipool:                fakePool,
			expectedIsStatusDirty: true,
			expectedState:         ofcirv1.StateAvailable,
			expectedRetryAfter:    defaultCirRetryDelay,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			fakeLogger := logr.New(log.NullLogSink{})

			fsm := NewCIResourceFSM(fakeLogger)
			resDirty, statusDirty, retryAfter, err := fsm.Process(tt.cir, tt.cipool, &corev1.Secret{})
			if !tt.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.expectedState, tt.cir.Status.State)
			assert.Equal(t, tt.expectedRetryAfter, retryAfter)
			assert.Equal(t, tt.expectedIsResourceDirty, resDirty, "Unexpected resource update")
			assert.Equal(t, tt.expectedIsStatusDirty, statusDirty, "Unexpected status update")
		})
	}
}
