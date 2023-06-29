package bootstrap

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	corev1alpha1 "package-operator.run/apis/core/v1alpha1"
	manifestsv1alpha1 "package-operator.run/apis/manifests/v1alpha1"
	"package-operator.run/package-operator/internal/testutil"
)

func TestBootstrapperBootstrap(t *testing.T) {
	c := testutil.NewClient()
	var initCalled bool
	b := &Bootstrapper{
		log:    testr.New(t),
		client: c,
		init: func(ctx context.Context) (
			*corev1alpha1.ClusterPackage, error,
		) {
			initCalled = true
			return &corev1alpha1.ClusterPackage{}, nil
		},
	}
	b.SetEnvironment(&manifestsv1alpha1.PackageEnvironment{
		Proxy: &manifestsv1alpha1.PackageEnvironmentProxy{
			HTTPProxy:  "httpxxx",
			HTTPSProxy: "httpsxxx",
			NoProxy:    "noxxx",
		},
	})

	c.On("Get", mock.Anything, mock.Anything,
		mock.AnythingOfType("*v1.CustomResourceDefinition"),
		mock.Anything).
		Return(errors.NewNotFound(schema.GroupResource{}, ""))

	c.On("Get", mock.Anything, mock.Anything,
		mock.AnythingOfType("*v1.Deployment"),
		mock.Anything).
		Run(func(args mock.Arguments) {
			depl := args.Get(2).(*appsv1.Deployment)
			depl.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionTrue,
				},
			}
		}).
		Return(nil)

	ctx := context.Background()
	err := b.Bootstrap(
		ctx, func(ctx context.Context) error { return nil })
	require.NoError(t, err)
	assert.True(t, initCalled)

	assert.Equal(t, os.Getenv("HTTP_PROXY"), "httpxxx")
	assert.Equal(t, os.Getenv("HTTPS_PROXY"), "httpsxxx")
	assert.Equal(t, os.Getenv("NO_PROXY"), "noxxx")
}

func TestBootstrapper_bootstrap(t *testing.T) {
	c := testutil.NewClient()
	b := &Bootstrapper{client: c}

	var (
		runManagerCalled bool
		runManagerCtx    context.Context
	)

	c.On("Get", mock.Anything, mock.Anything,
		mock.AnythingOfType("*v1.Deployment"),
		mock.Anything).
		Run(func(args mock.Arguments) {
			depl := args.Get(2).(*appsv1.Deployment)
			depl.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionTrue,
				},
			}
		}).
		Return(nil)

	ctx, cancel := context.WithTimeout(
		context.Background(), 2*time.Second)
	defer cancel()
	err := b.bootstrap(ctx, func(ctx context.Context) error {
		runManagerCalled = true
		runManagerCtx = ctx
		<-ctx.Done()
		return nil
	})
	require.NoError(t, err)
	assert.True(t, runManagerCalled)
	assert.Equal(t, context.Canceled, runManagerCtx.Err())
}

func TestBootstrapper_isPKOAvailable(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		c := testutil.NewClient()

		c.On("Get", mock.Anything, mock.Anything,
			mock.AnythingOfType("*v1.Deployment"),
			mock.Anything).
			Return(errors.NewNotFound(schema.GroupResource{}, ""))

		b := &Bootstrapper{client: c}
		isPKOAvailable, err := b.isPKOAvailable(
			context.Background())
		require.NoError(t, err)
		assert.False(t, isPKOAvailable)
	})

	t.Run("not available", func(t *testing.T) {
		c := testutil.NewClient()

		c.On("Get", mock.Anything, mock.Anything,
			mock.AnythingOfType("*v1.Deployment"),
			mock.Anything).
			Return(nil)

		b := &Bootstrapper{client: c}
		isPKOAvailable, err := b.isPKOAvailable(
			context.Background())
		require.NoError(t, err)
		assert.False(t, isPKOAvailable)
	})

	t.Run("available", func(t *testing.T) {
		c := testutil.NewClient()

		c.On("Get", mock.Anything, mock.Anything,
			mock.AnythingOfType("*v1.Deployment"),
			mock.Anything).
			Run(func(args mock.Arguments) {
				depl := args.Get(2).(*appsv1.Deployment)
				depl.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				}
			}).
			Return(nil)

		b := &Bootstrapper{client: c}
		isPKOAvailable, err := b.isPKOAvailable(
			context.Background())
		require.NoError(t, err)
		assert.True(t, isPKOAvailable)
	})
}

func TestBootstrapper_fixSliceCRDRenamingIssue(t *testing.T) {
	c := testutil.NewClient()
	b := &Bootstrapper{client: c}

	c.On("Get", mock.Anything, mock.Anything,
		mock.AnythingOfType("*v1.CustomResourceDefinition"),
		mock.Anything).
		Return(nil)

	c.On("Delete", mock.Anything,
		mock.AnythingOfType("*v1.Deployment"),
		mock.Anything).
		Return(nil)

	c.On("List", mock.Anything,
		mock.AnythingOfType("*v1alpha1.ClusterObjectSetList"),
		mock.Anything).
		Run(func(args mock.Arguments) {
			l := args.Get(1).(*corev1alpha1.ClusterObjectSetList)
			l.Items = []corev1alpha1.ClusterObjectSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{"xxx"},
					},
				}, {},
			}
		}).
		Return(nil)

	c.On("Get", mock.Anything, mock.Anything,
		mock.AnythingOfType("*v1alpha1.ClusterObjectSet"),
		mock.Anything).
		Return(errors.NewNotFound(schema.GroupResource{}, ""))

	c.On("Delete", mock.Anything,
		mock.AnythingOfType("*v1.CustomResourceDefinition"),
		mock.Anything).
		Return(nil)

	c.On("Delete", mock.Anything,
		mock.AnythingOfType("*v1alpha1.ClusterObjectSet"),
		mock.Anything).
		Return(nil)

	c.On("Update", mock.Anything,
		mock.AnythingOfType("*v1alpha1.ClusterObjectSet"),
		mock.Anything).
		Return(nil)

	err := b.fixSliceCRDRenamingIssue(context.Background())
	require.NoError(t, err)

	c.AssertExpectations(t)
}
