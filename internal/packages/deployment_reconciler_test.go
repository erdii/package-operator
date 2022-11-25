package packages

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "package-operator.run/apis/core/v1alpha1"
	"package-operator.run/package-operator/internal/adapters"
	"package-operator.run/package-operator/internal/testutil"
)

func Test_DeploymentReconciler_Reconcile(t *testing.T) {
	c := testutil.NewClient()
	r := newDeploymentReconciler(testScheme, c,
		adapters.NewObjectDeployment,
		adapters.NewObjectSlice,
		adapters.NewObjectSliceList,
		newGenericObjectSetList)
	ctx := logr.NewContext(context.Background(), testr.New(t))

	deploy := &adapters.ObjectDeployment{
		ObjectDeployment: corev1alpha1.ObjectDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-depl",
			},
			Spec: corev1alpha1.ObjectDeploymentSpec{
				Template: corev1alpha1.ObjectSetTemplate{
					Spec: corev1alpha1.ObjectSetTemplateSpec{
						Phases: []corev1alpha1.ObjectSetTemplatePhase{
							{
								Name: "test",
								Objects: []corev1alpha1.ObjectSetObject{
									{
										Object: unstructured.Unstructured{},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.
		On("Get",
			mock.Anything,
			mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectDeployment"),
			mock.Anything,
		).
		Once().
		Return(errors.NewNotFound(schema.GroupResource{}, ""))
	var createdDeployment *corev1alpha1.ObjectDeployment
	c.
		On("Create", mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectDeployment"),
			mock.Anything).
		Run(func(args mock.Arguments) {
			createdDeployment = args.Get(1).(*corev1alpha1.ObjectDeployment).DeepCopy()
		}).
		Return(nil)
	var createdSlice *corev1alpha1.ObjectSlice
	c.
		On("Create", mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectSlice"),
			mock.Anything).
		Run(func(args mock.Arguments) {
			createdSlice = args.Get(1).(*corev1alpha1.ObjectSlice).DeepCopy()
		}).
		Return(nil)

		// retries on conflict
	c.
		On("Update", mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectDeployment"),
			mock.Anything).
		Once().
		Return(errors.NewConflict(schema.GroupResource{}, "", nil))
	c.
		On("Get",
			mock.Anything,
			mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectDeployment"),
			mock.Anything,
		).
		Return(nil)

	var updatedDeployment *corev1alpha1.ObjectDeployment
	c.
		On("Update", mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectDeployment"),
			mock.Anything).
		Run(func(args mock.Arguments) {
			updatedDeployment = args.Get(1).(*corev1alpha1.ObjectDeployment).DeepCopy()
		}).
		Return(nil)
	c.
		On("List", mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectSetList"),
			mock.Anything).
		Return(nil)
	c.
		On("List", mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectSliceList"),
			mock.Anything).
		Return(nil)

	err := r.Reconcile(ctx, deploy, &EachObjectChunker{})
	require.NoError(t, err)

	// ObjectDeployment is created empty.
	assert.Empty(t, createdDeployment.Spec.Template.Spec.Phases)

	assert.Equal(t, []corev1alpha1.ObjectSetObject{
		{
			Object: unstructured.Unstructured{},
		},
	}, createdSlice.Objects)

	assert.Equal(t, []corev1alpha1.ObjectSetTemplatePhase{
		{
			Name:   "test",
			Slices: []string{"test-depl-754bcb585"},
		},
	}, updatedDeployment.Spec.Template.Spec.Phases)
}

func TestDeploymentReconciler_reconcileSlice_hashCollision(t *testing.T) {
	c := testutil.NewClient()
	r := newDeploymentReconciler(testScheme, c,
		adapters.NewObjectDeployment,
		adapters.NewObjectSlice,
		adapters.NewObjectSliceList,
		newGenericObjectSetList)
	ctx := logr.NewContext(context.Background(), testr.New(t))

	deploy := &adapters.ObjectDeployment{
		ObjectDeployment: corev1alpha1.ObjectDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-depl",
			},
		},
	}

	slice := &adapters.ObjectSlice{
		ObjectSlice: corev1alpha1.ObjectSlice{
			Objects: []corev1alpha1.ObjectSetObject{
				{
					Object: unstructured.Unstructured{},
				},
			},
		},
	}

	c.On("Create",
		mock.Anything,
		mock.AnythingOfType("*v1alpha1.ObjectSlice"),
		mock.Anything).
		Once().
		Return(errors.NewAlreadyExists(schema.GroupResource{}, ""))

	c.On("Create",
		mock.Anything,
		mock.AnythingOfType("*v1alpha1.ObjectSlice"),
		mock.Anything).
		Return(nil)

	c.On("Get",
		mock.Anything,
		mock.Anything,
		mock.AnythingOfType("*v1alpha1.ObjectSlice"),
		mock.Anything).
		Run(func(args mock.Arguments) {
			slice := args.Get(2).(*corev1alpha1.ObjectSlice)
			*slice = corev1alpha1.ObjectSlice{
				Objects: []corev1alpha1.ObjectSetObject{
					{
						Object: unstructured.Unstructured{},
					},
				},
			}
		}).
		Return(nil)

	err := r.reconcileSlice(ctx, deploy, slice)
	require.NoError(t, err)

	c.AssertNumberOfCalls(t, "Create", 2)
}

func TestDeploymentReconciler_sliceGarbageCollection(t *testing.T) {
	c := testutil.NewClient()
	r := newDeploymentReconciler(testScheme, c,
		adapters.NewObjectDeployment,
		adapters.NewObjectSlice,
		adapters.NewObjectSliceList,
		newGenericObjectSetList)
	ctx := logr.NewContext(context.Background(), testr.New(t))

	deploy := &adapters.ObjectDeployment{
		ObjectDeployment: corev1alpha1.ObjectDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-depl",
			},
			Spec: corev1alpha1.ObjectDeploymentSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
				Template: corev1alpha1.ObjectSetTemplate{
					Spec: corev1alpha1.ObjectSetTemplateSpec{
						Phases: []corev1alpha1.ObjectSetTemplatePhase{
							{
								Slices: []string{"slice0-xxx"},
							},
						},
					},
				},
			},
		},
	}

	objectSet1 := &corev1alpha1.ObjectSet{
		Spec: corev1alpha1.ObjectSetSpec{
			ObjectSetTemplateSpec: corev1alpha1.ObjectSetTemplateSpec{
				Phases: []corev1alpha1.ObjectSetTemplatePhase{
					{
						Name:   "test",
						Slices: []string{"slice1-xxx"},
					},
				},
			},
		},
	}

	objectSlice0 := &corev1alpha1.ObjectSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "slice0-xxx",
		},
	}
	objectSlice1 := &corev1alpha1.ObjectSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "slice1-xxx",
		},
	}
	objectSlice2 := &corev1alpha1.ObjectSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "slice2-xxx",
		},
	}

	c.
		On("List",
			mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectSetList"),
			mock.Anything).
		Run(func(args mock.Arguments) {
			list := args.Get(1).(*corev1alpha1.ObjectSetList)
			list.Items = []corev1alpha1.ObjectSet{
				*objectSet1,
			}
		}).
		Return(nil)
	c.
		On("List",
			mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectSliceList"),
			mock.Anything).
		Run(func(args mock.Arguments) {
			list := args.Get(1).(*corev1alpha1.ObjectSliceList)
			list.Items = []corev1alpha1.ObjectSlice{
				*objectSlice0, *objectSlice1, *objectSlice2,
			}
		}).
		Return(nil)
	c.
		On("Delete",
			mock.Anything,
			mock.AnythingOfType("*v1alpha1.ObjectSlice"),
			mock.Anything).
		Return(nil)

	err := r.sliceGarbageCollection(ctx, deploy)
	require.NoError(t, err)

	c.AssertNumberOfCalls(t, "Delete", 1)
	c.AssertCalled(
		t, "Delete", mock.Anything, objectSlice2, mock.Anything)
}

func Test_sliceCollisionError(t *testing.T) {
	e := &sliceCollisionError{
		key: client.ObjectKey{
			Name: "test", Namespace: "test",
		},
	}

	assert.Equal(t, "ObjectSlice collision with test/test", e.Error())
}
