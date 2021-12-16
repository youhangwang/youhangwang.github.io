package main

import (
	"context"
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	jsonPatch := []byte(`{
		"metadata":{
			"annotations": {
				"provider-name": "my-provider-update"
			}
		}
	}
	`)

	clientset, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.MergePatchType, jsonPatch, metav1.PatchOptions{})
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	fmt.Print("Patched Deployment %+v", deployment)
}
