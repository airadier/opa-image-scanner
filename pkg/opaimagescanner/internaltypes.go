package opaimagescanner

import (
	"image-scan-webhook/pkg/imagescanner"
	"image-scan-webhook/pkg/opa"
)

const regoQuery string = "data.imageadmission.deny_image"

const regoDefaultRules string = `
package imageadmission

deny_image[msg] {
	msg := "No rules defined. Please define 'imageadmission' package with deny_image[msg] rules"
}
`

type opaImageScannerEvaluator struct {
	scanner         imagescanner.Scanner
	opaEvaluator    opa.OPAEvaluator
	getOPARulesFunc GetStringDataFunction
	getOPADataFunc  GetStringDataFunction
}

// Verify that opaImageScannerEvaluator implements ImageScannerAdmissionEvaluator.
var _ imagescanner.ImageScannerAdmissionEvaluator = (*opaImageScannerEvaluator)(nil)
