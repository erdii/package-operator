package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"pkg.package-operator.run/cardboard/kubeutils/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"package-operator.run/apis"
	corev1alpha1 "package-operator.run/apis/core/v1alpha1"
)

var (
	s *runtime.Scheme
	c client.Client
	w *wait.Waiter
)

func init() {
	s = runtime.NewScheme()
	must(scheme.AddToScheme(s))
	must(apis.AddToScheme(s))

	restConfig := config.GetConfigOrDie()

	var err error
	c, err = client.New(restConfig, client.Options{
		Scheme: s,
	})
	must(err)

	w = wait.NewWaiter(c, s)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func newConfigMap(index int) unstructured.Unstructured {
	name := fmt.Sprintf("test-%d", index)
	cmObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(
		&corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Data: map[string]string{
				"banana": "bread",
				"name":   name,
			},
		},
	)
	must(err)
	return unstructured.Unstructured{
		Object: cmObj,
	}
}

func newObjectSet(n int) *corev1alpha1.ObjectSet {
	objects := make([]corev1alpha1.ObjectSetObject, 0, n)
	for i := range n {
		objects = append(objects, corev1alpha1.ObjectSetObject{
			Object: newConfigMap(i),
		})
	}

	return &corev1alpha1.ObjectSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-1",
			Namespace: "default",
		},
		Spec: corev1alpha1.ObjectSetSpec{
			ObjectSetTemplateSpec: corev1alpha1.ObjectSetTemplateSpec{
				Phases: []corev1alpha1.ObjectSetTemplatePhase{
					{
						Name:    "one",
						Objects: objects,
					},
				},
			},
		},
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	const n = 20

	for {
		fmt.Println("tick", time.Now())

		objectSet := newObjectSet(n)
		must(c.Create(ctx, objectSet))
		must(w.WaitForCondition(
			ctx,
			objectSet,
			corev1alpha1.ObjectSetAvailable,
			metav1.ConditionTrue))
		must(c.Delete(ctx, objectSet,
			client.PropagationPolicy(metav1.DeletePropagationOrphan)))
		must(w.WaitToBeGone(ctx, objectSet,
			func(_ client.Object) (done bool, err error) {
				return false, nil
			}))

		cmList := &corev1.ConfigMapList{}
		must(c.List(ctx, cmList,
			client.InNamespace("default"),
			client.HasLabels{"package-operator.run/cache"}))
		if len(cmList.Items) != 20 {
			panic(fmt.Sprintf(
				"cmList contains %d instead of %d items", len(cmList.Items), n))
		}
		for _, cm := range cmList.Items {
			if len(cm.OwnerReferences) != 0 {
				panic(fmt.Sprintf("cm %s contains ownerReferences", cm.Name))
			}
		}

		must(c.DeleteAllOf(ctx, &corev1.ConfigMap{},
			client.InNamespace("default"),
			client.HasLabels{"package-operator.run/cache"}))
	}
}
