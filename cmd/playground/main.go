package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"package-operator.run/apis"
	corev1alpha1 "package-operator.run/apis/core/v1alpha1"
)

const pkoClusterObjectSetLabelSelector = "package-operator.run/package=package-operator"

func main() {
	cl, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		panic(fmt.Errorf("could not create client: %w", err))
	}

	err = apis.AddToScheme(cl.Scheme())
	if err != nil {
		panic(fmt.Errorf("could not register pko apis into scheme: %w", err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pkoCosLabelSelector := client.MatchingLabelsSelector{
		Selector: mustParseLabelSelector(pkoClusterObjectSetLabelSelector),
	}

	fmt.Println("deleting") //nolint:forbidigo
	err = cl.DeleteAllOf(
		ctx,
		&corev1alpha1.ClusterObjectSet{},
		pkoCosLabelSelector,
		client.PropagationPolicy(metav1.DeletePropagationOrphan),
	)
	if err != nil {
		panic(fmt.Errorf("could not deleteAllOf: %w", err))
	}
	fmt.Println("deleted") //nolint:forbidigo

	fmt.Println("listing") //nolint:forbidigo
	list := &corev1alpha1.ClusterObjectSetList{}
	err = cl.List(ctx, list, pkoCosLabelSelector)
	if err != nil {
		panic(fmt.Errorf("could not list cos: %w", err))
	}
	fmt.Println("listed") //nolint:forbidigo

	for i := range list.Items {
		cos := list.Items[i]
		fmt.Printf("\tpatching: %s\n", cos.Name) //nolint:forbidigo
		patch := client.MergeFrom(cos.DeepCopy())
		cos.ObjectMeta.Finalizers = []string{}
		err := cl.Patch(ctx, &cos, patch)
		if err != nil {
			panic(fmt.Errorf("could not patch cos %s: %w", cos.Name, err))
		}
		fmt.Printf("\tpatched\n") //nolint:forbidigo
	}

	fmt.Printf("done") //nolint:forbidigo
}

func mustParseLabelSelector(selector string) labels.Selector {
	labelSelector, err := labels.Parse(selector)
	if err != nil {
		panic(fmt.Errorf("must be able to parse label selector string: %s, %w", selector, err))
	}
	return labelSelector
}
