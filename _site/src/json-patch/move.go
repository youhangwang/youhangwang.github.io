package main

import (
	"context"
	"encoding/json"
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

type JsonPatch []PatchOperation
type PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	From  string      `json:"from,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

func main() {
	jsonPatch := JsonPatch{
		PatchOperation{
			Op:   "move",
			Path: "/metadata/annotations/provider-name",
			From: "/metadata/labels/provider-name",
		},
	}

	patchByte, err := json.Marshal(jsonPatch)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	clientset, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	deployment, err := clientset.AppsV1().Deployments(apiv1.NamespaceDefault).Patch(context.TODO(), "my-deployment", types.JSONPatchType, patchByte, metav1.PatchOptions{})
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	fmt.Print("Patched Deployment %+v", deployment)
}
