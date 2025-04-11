// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package cmdutils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"

	bootv1alphav1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Move transfers all BootOperator-related CRs from a source to a target cluster.
// It verifies object equality and handles secrets and owner references.
func Move(
	ctx context.Context,
	clients Clients,
	scheme *runtime.Scheme,
	namespace string,
	requireOwners bool,
	dryRun bool,
) error {
	httpConfigs, ipxeConfigs, err := getCrs(ctx, clients.Source, namespace)
	if err != nil {
		return err
	}
	slog.Debug(fmt.Sprintf("found %s CRs in the source cluster", bootv1alphav1.GroupVersion.Group),
		slog.Any("http boot configs", transform(httpConfigs, httpBootConfigName)),
		slog.Any("ipxe boot configs", transform(ipxeConfigs, ipxeBootConfigName)))

	objsToMove, err := getObjsToBeMoved(ctx, clients, httpConfigs, ipxeConfigs)
	if err != nil {
		return err
	}
	slog.Debug("moving", slog.Any("objects", transform(objsToMove, objName)))

	if !dryRun {
		var movedObjs []client.Object
		if movedObjs, err = moveObjs(ctx, clients.Target, scheme, objsToMove, requireOwners); err != nil {
			cleanupErr := cleanup(ctx, clients.Target, movedObjs)
			err = errors.Join(err,
				fmt.Errorf("clean up of already moved objects was performed to restore a target cluster's state with error result: %w",
					cleanupErr))
		} else {
			slog.Debug(fmt.Sprintf("all %s CRs with theirs secrets from the source cluster were moved to the target cluster",
				bootv1alphav1.GroupVersion.Group))
		}
	}

	return err
}

func getCrs(
	ctx context.Context,
	cl client.Client,
	namespace string,
) ([]bootv1alphav1.HTTPBootConfig, []bootv1alphav1.IPXEBootConfig, error) {
	httpBootConfigList := &bootv1alphav1.HTTPBootConfigList{}
	if err := cl.List(ctx, httpBootConfigList, &client.ListOptions{Namespace: namespace}); err != nil {
		return nil, nil, fmt.Errorf("couldn't list HTTPBootConfigs: %w", err)
	}
	IPXEBootConfigList := &bootv1alphav1.IPXEBootConfigList{}
	if err := cl.List(ctx, IPXEBootConfigList, &client.ListOptions{Namespace: namespace}); err != nil {
		return nil, nil, fmt.Errorf("couldn't list IPXEBootConfigs: %w", err)
	}
	return httpBootConfigList.Items, IPXEBootConfigList.Items, nil
}

func getObjsToBeMoved(
	ctx context.Context,
	clients Clients,
	sourceHttpCrs []bootv1alphav1.HTTPBootConfig,
	sourceIpxeCrs []bootv1alphav1.IPXEBootConfig,
) ([]client.Object, error) {
	objsToMove := make([]client.Object, 0, 2*(len(sourceHttpCrs)+len(sourceIpxeCrs)))
	uidToSecretNameMap := getUidToSecretNameMap(sourceHttpCrs, sourceIpxeCrs)
	httpObjs := transform(sourceHttpCrs, func(c bootv1alphav1.HTTPBootConfig) client.Object { return &c })
	ipxeObjs := transform(sourceIpxeCrs, func(c bootv1alphav1.IPXEBootConfig) client.Object { return &c })

	for _, sourceObj := range slices.Concat(httpObjs, ipxeObjs) {
		sourceObjNN := client.ObjectKeyFromObject(sourceObj)
		targetObj := sourceObj.DeepCopyObject().(client.Object)

		err := clients.Target.Get(ctx, sourceObjNN, targetObj)
		if apierrors.IsNotFound(err) {
			objsToMove = append(objsToMove, sourceObj)

			secret, err := getSecret(ctx, clients, uidToSecretNameMap[sourceObj.GetUID()], sourceObj.GetNamespace())
			if err != nil {
				return nil, err
			} else if secret != nil {
				objsToMove = append(objsToMove, secret)
			}
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("failed to check an object existence in the target cluster: %w", err)
		}

		if reflect.DeepEqual(clearFields(sourceObj), clearFields(targetObj)) {
			slog.Debug("source and target objects are the same", slog.String("object", sourceObjNN.String()))
			continue
		}
		return nil, fmt.Errorf(
			"a %q object already exists in the target cluster and is different than in the source cluster",
			sourceObjNN.String())
	}
	return objsToMove, nil
}

func getUidToSecretNameMap(
	sourceHttpCrs []bootv1alphav1.HTTPBootConfig,
	sourceIpxeCrs []bootv1alphav1.IPXEBootConfig,
) map[types.UID]string {
	uidToSecretNameMap := map[types.UID]string{}
	for _, config := range sourceHttpCrs {
		if config.Spec.IgnitionSecretRef != nil {
			uidToSecretNameMap[config.UID] = config.Spec.IgnitionSecretRef.Name
		}
	}
	for _, config := range sourceIpxeCrs {
		if config.Spec.IgnitionSecretRef != nil {
			uidToSecretNameMap[config.UID] = config.Spec.IgnitionSecretRef.Name
		}
	}
	return uidToSecretNameMap
}

func getSecret(
	ctx context.Context,
	clients Clients,
	name string,
	namespace string,
) (*v1.Secret, error) {
	if name == "" {
		return nil, nil
	}

	nn := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	sourceSecret := &v1.Secret{}
	err := clients.Source.Get(ctx, nn, sourceSecret)
	if err != nil {
		return nil, fmt.Errorf("can't get a %q secret in the source cluster: %w", nn.String(), err)
	}

	targetSecret := &v1.Secret{}
	err = clients.Target.Get(ctx, nn, targetSecret)
	if apierrors.IsNotFound(err) {
		return sourceSecret, nil
	} else if err != nil {
		return nil, fmt.Errorf("can't get a %q secret in the target cluster: %w", nn.String(), err)
	}

	if reflect.DeepEqual(clearFields(sourceSecret), clearFields(targetSecret)) {
		slog.Debug("source and target secrets are the same", slog.String("secret", nn.String()))
		return nil, nil
	}
	return nil, fmt.Errorf(
		"a %q secret already exists in the target cluster and is different than in the source cluster",
		nn.String())
}

func clearFields(obj client.Object) client.Object {
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetGeneration(0)
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetManagedFields(nil)
	obj.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{})
	return obj
}

func moveObjs(
	ctx context.Context,
	cl client.Client,
	scheme *runtime.Scheme,
	objs []client.Object,
	requireOwners bool,
) ([]client.Object, error) {
	movedObjs := make([]client.Object, 0, len(objs))

	for _, obj := range objs {
		objKey := client.ObjectKeyFromObject(obj)
		if obj.GetObjectKind().GroupVersionKind().Group == bootv1alphav1.GroupVersion.Group && len(obj.GetOwnerReferences()) == 1 {
			obj.SetOwnerReferences([]metav1.OwnerReference{})
			serverBootConfiguration := &metalv1alpha1.ServerBootConfiguration{}

			err := cl.Get(ctx, objKey, serverBootConfiguration)
			if err == nil {
				err = controllerutil.SetControllerReference(serverBootConfiguration, obj, scheme)
				if err != nil {
					return movedObjs, fmt.Errorf("error when setting owner reference: %w", err)
				}
			} else if apierrors.IsNotFound(err) && requireOwners {
				return movedObjs, fmt.Errorf("an owner ServerBootConfiguration %q wasn't found in the target cluster: %w", objKey.String(), err)
			} else if !apierrors.IsNotFound(err) {
				return movedObjs, fmt.Errorf("error when getting a server boot configuration: %w", err)
			}
		}

		obj.SetResourceVersion("")
		if err := cl.Create(ctx, obj); err != nil {
			err = fmt.Errorf("object %q couldn't be created in the target cluster: %w", objKey.String(), err)
			return movedObjs, err
		}
		movedObjs = append(movedObjs, obj)
	}

	return movedObjs, nil
}

func cleanup(ctx context.Context, cl client.Client, objs []client.Object) error {
	cleanupErrs := make([]error, 0)
	for _, obj := range objs {
		if err := cl.Delete(ctx, obj); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	return errors.Join(cleanupErrs...)
}
