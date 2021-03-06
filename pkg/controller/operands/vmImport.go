package operands

import (
	"errors"
	"os"
	"reflect"

	vmimportv1beta1 "github.com/kubevirt/vm-import-operator/pkg/apis/v2v/v1beta1"
	conditionsv1 "github.com/openshift/custom-resource-status/conditions/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hcov1beta1 "github.com/kubevirt/hyperconverged-cluster-operator/pkg/apis/hco/v1beta1"
	"github.com/kubevirt/hyperconverged-cluster-operator/pkg/controller/common"
	"github.com/kubevirt/hyperconverged-cluster-operator/pkg/util"
	hcoutil "github.com/kubevirt/hyperconverged-cluster-operator/pkg/util"
)

// ***********  VM Import Handler  ***********
type vmImportHandler genericOperand

func newVmImportHandler(Client client.Client, Scheme *runtime.Scheme) *vmImportHandler {
	return &vmImportHandler{
		Client: Client,
		Scheme: Scheme,
		crType: "vmImport",
		// Previous versions used to have HCO-operator (scope namespace)
		// as the owner of VMImportConfig (scope cluster).
		// It's not legal, so remove that.
		removeExistingOwner: true,
		hooks:               &vmImportHooks{},
	}
}

type vmImportHooks struct {
	cache *vmimportv1beta1.VMImportConfig
}

func (h *vmImportHooks) getFullCr(hc *hcov1beta1.HyperConverged) (client.Object, error) {
	if h.cache == nil {
		h.cache = NewVMImportForCR(hc)
	}
	return h.cache, nil
}
func (h vmImportHooks) getEmptyCr() client.Object                              { return &vmimportv1beta1.VMImportConfig{} }
func (h vmImportHooks) postFound(_ *common.HcoRequest, _ runtime.Object) error { return nil }
func (h vmImportHooks) getConditions(cr runtime.Object) []conditionsv1.Condition {
	return cr.(*vmimportv1beta1.VMImportConfig).Status.Conditions
}
func (h vmImportHooks) checkComponentVersion(cr runtime.Object) bool {
	found := cr.(*vmimportv1beta1.VMImportConfig)
	return checkComponentVersion(hcoutil.VMImportEnvV, found.Status.ObservedVersion)
}
func (h vmImportHooks) getObjectMeta(cr runtime.Object) *metav1.ObjectMeta {
	return &cr.(*vmimportv1beta1.VMImportConfig).ObjectMeta
}
func (h *vmImportHooks) reset() {
	h.cache = nil
}

func (h *vmImportHooks) updateCr(req *common.HcoRequest, Client client.Client, exists runtime.Object, required runtime.Object) (bool, bool, error) {
	vmImport, ok1 := required.(*vmimportv1beta1.VMImportConfig)
	found, ok2 := exists.(*vmimportv1beta1.VMImportConfig)

	if !ok1 || !ok2 {
		return false, false, errors.New("can't convert to vmImport")
	}

	if !reflect.DeepEqual(found.Spec, vmImport.Spec) ||
		!reflect.DeepEqual(found.Labels, vmImport.Labels) {
		if req.HCOTriggered {
			req.Logger.Info("Updating existing vmImport's Spec to new opinionated values")
		} else {
			req.Logger.Info("Reconciling an externally updated vmImport's Spec to its opinionated values")
		}
		util.DeepCopyLabels(&vmImport.ObjectMeta, &found.ObjectMeta)
		vmImport.Spec.DeepCopyInto(&found.Spec)
		err := Client.Update(req.Ctx, found)
		if err != nil {
			return false, false, err
		}
		return true, !req.HCOTriggered, nil
	}

	return false, false, nil
}

// NewVMImportForCR returns a VM import CR
func NewVMImportForCR(cr *hcov1beta1.HyperConverged) *vmimportv1beta1.VMImportConfig {
	spec := vmimportv1beta1.VMImportConfigSpec{}
	if cr.Spec.Infra.NodePlacement != nil {
		cr.Spec.Infra.NodePlacement.DeepCopyInto(&spec.Infra)
	}
	return &vmimportv1beta1.VMImportConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "vmimport-" + cr.Name,
			Labels: getLabels(cr, hcoutil.AppComponentImport),
		},
		Spec: spec,
	}
}

// ************** IMS Config Handler **************
type imsConfigHandler genericOperand

func newImsConfigHandler(Client client.Client, Scheme *runtime.Scheme) *imsConfigHandler {
	return &imsConfigHandler{
		Client:                 Client,
		Scheme:                 Scheme,
		crType:                 "IMSConfigmap",
		removeExistingOwner:    false,
		setControllerReference: true,
		hooks:                  &imsConfigHooks{},
	}
}

type imsConfigHooks struct{}

func (h imsConfigHooks) getFullCr(hc *hcov1beta1.HyperConverged) (client.Object, error) {
	return NewIMSConfigForCR(hc, hc.Namespace)
}
func (h imsConfigHooks) getEmptyCr() client.Object { return &corev1.ConfigMap{} }

func (h imsConfigHooks) postFound(_ *common.HcoRequest, _ runtime.Object) error { return nil }
func (h imsConfigHooks) getObjectMeta(cr runtime.Object) *metav1.ObjectMeta {
	return &cr.(*corev1.ConfigMap).ObjectMeta
}

func (h *imsConfigHooks) updateCr(req *common.HcoRequest, Client client.Client, exists runtime.Object, required runtime.Object) (bool, bool, error) {
	imsConfig, ok1 := required.(*corev1.ConfigMap)
	found, ok2 := exists.(*corev1.ConfigMap)
	if !ok1 || !ok2 {
		return false, false, errors.New("can't convert to a ConfigMap")
	}

	needsUpdate := false

	if !reflect.DeepEqual(found.Data, imsConfig.Data) {
		imsConfig.DeepCopyInto(found)
		needsUpdate = true
	}

	if !reflect.DeepEqual(found.Labels, imsConfig.Labels) {
		util.DeepCopyLabels(&imsConfig.ObjectMeta, &found.ObjectMeta)
		needsUpdate = true
	}

	if needsUpdate {
		req.Logger.Info("Updating existing IMS Configmap to its default values")

		if req.HCOTriggered {
			req.Logger.Info("Updating existing IMS Configmap to new opinionated values")
		} else {
			req.Logger.Info("Reconciling an externally updated IMS Configmap to its opinionated values")
		}

		err := Client.Update(req.Ctx, found)
		if err != nil {
			return false, false, err
		}
		return true, !req.HCOTriggered, nil
	}

	return false, false, nil
}

func NewIMSConfigForCR(cr *hcov1beta1.HyperConverged, namespace string) (*corev1.ConfigMap, error) {
	conversionContainer := os.Getenv("CONVERSION_CONTAINER")
	if conversionContainer == "" {
		return nil, errors.New("ims-conversion-container not specified")
	}

	vmwareContainer := os.Getenv("VMWARE_CONTAINER")
	if vmwareContainer == "" {
		return nil, errors.New("ims-vmware-container not specified")
	}

	virtiowinContainer := os.Getenv("VIRTIOWIN_CONTAINER")
	if virtiowinContainer == "" {
		return nil, errors.New("kv-virtiowin-image-name not specified")
	}

	imscm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "v2v-vmware",
			Labels:    getLabels(cr, hcoutil.AppComponentImport),
			Namespace: namespace,
		},
		Data: map[string]string{
			"v2v-conversion-image":              conversionContainer,
			"kubevirt-vmware-image":             vmwareContainer,
			"virtio-win-image":                  virtiowinContainer,
			"kubevirt-vmware-image-pull-policy": "IfNotPresent",
		},
	}
	if cr.Spec.VddkInitImage != nil {
		imscm.Data["vddk-init-image"] = *cr.Spec.VddkInitImage
	}
	return imscm, nil
}
