package opaimagescanner

import (
	"encoding/json"
	"fmt"
	"image-scan-webhook/pkg/imagescanner"
	"image-scan-webhook/pkg/opa"
	"io/ioutil"
	"strings"
	"testing"

	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

type StartScanReturn struct {
	Digest string
	Error  error
}

type GetReportReturn struct {
	Report *imagescanner.ScanReport
	Error  error
}

//TODO: Check go-mock
type mockImageScanner struct {
	T *testing.T

	ExpectedImageAndTag string
	ExpectedImageDigest string

	StartScanReturn StartScanReturn
	GetReportReturn GetReportReturn

	StartScanCalled bool
	GetReportCalled bool
}

var pod = &corev1.Pod{
	Spec: corev1.PodSpec{Containers: []corev1.Container{
		{Image: "mysaferegistry.io/container-image:1.01"},
	}},
}

func (s *mockImageScanner) StartScan(imageAndTag string) (string, error) {
	s.StartScanCalled = true

	if s.ExpectedImageAndTag != "" && s.ExpectedImageAndTag != imageAndTag {
		s.T.Fatalf("StartScan called with unexpected imageAndTag:\n%s", imageAndTag)
	}

	return s.StartScanReturn.Digest, s.StartScanReturn.Error
}

func (s *mockImageScanner) GetReport(imageAndTag, imageDigest string) (*imagescanner.ScanReport, error) {
	s.GetReportCalled = true

	if s.ExpectedImageAndTag != "" && s.ExpectedImageAndTag != imageAndTag {
		s.T.Fatalf("GetReport called with unexpected imageAndTag:\n%s", imageAndTag)
	}

	return s.GetReportReturn.Report, s.GetReportReturn.Error
}

// Verify that mockImageScanner implements imagescanner.Scanner
var _ imagescanner.Scanner = (*mockImageScanner)(nil)

type mockOPAEvaluator struct {
	T *testing.T

	ExpectedQuery string
	ExpectedRules string
	ExpectedData  string

	ReceivedInput interface{}

	ReturnError error
	Called      bool
}

func (e *mockOPAEvaluator) Evaluate(query string, rules, data string, input interface{}) error {
	e.Called = true

	if e.ExpectedQuery != "" && query != e.ExpectedQuery {
		e.T.Fatalf("OPAEvaluator.Evaluate called with unexpected query:\n%s", query)
	}

	if e.ExpectedRules != "" && rules != e.ExpectedRules {
		e.T.Fatalf("OPAEvaluator.Evaluate called with unexpected rules:\n%s", rules)
	}

	if e.ExpectedData != "" && data != e.ExpectedData {
		e.T.Fatalf("OPAEvaluator.Evaluate called with unexpected data:\n%s", data)
	}

	e.ReceivedInput = input

	return e.ReturnError
}

// Verify that mockEvaluator implements opa.OPAEvaluator
var _ opa.OPAEvaluator = (*mockOPAEvaluator)(nil)

func mockGetOPARules() (string, error) {
	return "package mock\nmock_rules{}", nil
}

func mockGetOPAData() (string, error) {
	return `{"mockData": true}`, nil
}

var mockRules, _ = mockGetOPARules()
var mockData, _ = mockGetOPAData()

func TestDummy(t *testing.T) {

	report := &imagescanner.ScanReport{
		Status: imagescanner.StatusAccepted,
	}

	scanner := &mockImageScanner{
		T:                   t,
		ExpectedImageAndTag: "mysaferegistry.io/container-image:1.01",
		ExpectedImageDigest: "TestDigest",
		StartScanReturn:     StartScanReturn{Digest: "TestDigest", Error: nil},
		GetReportReturn:     GetReportReturn{Report: report, Error: nil}}

	opaEvaluator := &mockOPAEvaluator{
		T:             t,
		ExpectedQuery: "data.imageadmission.deny_image",
		ExpectedRules: mockRules,
		ExpectedData:  mockData,
	}

	evaluator := NewImageScannerEvaluator(scanner, opaEvaluator, mockGetOPARules, mockGetOPAData)

	a := loadAdmissionRequest("./assets/admission-review.json", t)

	accepted, digestMappings, err := evaluator.ScanAndEvaluate(a, pod)
	if !accepted {
		t.Error(err)
	}

	if digestMappings["mysaferegistry.io/container-image:1.01"] != "TestDigest" {
		t.Fatalf("Unexpected digest mapping: %v", digestMappings)
	}

	if !scanner.StartScanCalled {
		t.Fatalf("StartScan was not called")
	}

	if !scanner.GetReportCalled {
		t.Fatalf("GetReportCalled was not called")
	}

	if !opaEvaluator.Called {
		t.Fatalf("OPAEvaluator.Evaluate was not called")
	}
}

func TestNilAdmissionReview(t *testing.T) {

	scanner := &mockImageScanner{}
	opaEvaluator := &mockOPAEvaluator{T: t}

	evaluator := NewImageScannerEvaluator(scanner, opaEvaluator, mockGetOPARules, mockGetOPAData)

	accepted, _, err := evaluator.ScanAndEvaluate(nil, nil)
	if accepted || len(err) != 1 || err[0] != "Admission request is <nil>" {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestEmptyAdmissionReview(t *testing.T) {
	report := &imagescanner.ScanReport{
		Status: imagescanner.StatusAccepted,
	}

	scanner := &mockImageScanner{
		StartScanReturn: StartScanReturn{Digest: "", Error: fmt.Errorf("Some error - forced in test")},
		GetReportReturn: GetReportReturn{Report: report, Error: nil}}
	opaEvaluator := &mockOPAEvaluator{T: t}

	evaluator := NewImageScannerEvaluator(scanner, opaEvaluator, mockGetOPARules, mockGetOPAData)
	a := &v1beta1.AdmissionRequest{}

	accepted, _, err := evaluator.ScanAndEvaluate(a, nil)
	if accepted || len(err) != 1 || err[0] != "Pod data is <nil>" {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestStartScanFails(t *testing.T) {
	report := &imagescanner.ScanReport{
		Status: imagescanner.StatusAccepted,
	}

	scanner := &mockImageScanner{
		StartScanReturn: StartScanReturn{Digest: "", Error: fmt.Errorf("Some error")},
		GetReportReturn: GetReportReturn{Report: report, Error: nil}}
	opaEvaluator := &mockOPAEvaluator{T: t}

	evaluator := NewImageScannerEvaluator(scanner, opaEvaluator, mockGetOPARules, mockGetOPAData)

	a := loadAdmissionRequest("./assets/admission-review.json", t)

	accepted, _, err := evaluator.ScanAndEvaluate(a, pod)
	if !accepted {
		t.Error(err)
	}

	if !scanner.StartScanCalled {
		t.Fatalf("StartScan was not called")
	}

	if scanner.GetReportCalled {
		t.Fatalf("GetReportCalled should NOT be called")
	}

	if !opaEvaluator.Called {
		t.Fatalf("OPAEvaluator.Evaluate was NOT called")
	}

	input := opaEvaluator.ReceivedInput.(OPAInput)
	if input.ScanReport.Status != imagescanner.StatusScanFailed {
		t.Fatalf("OPAEvaluator.Evaluate did not receive a Scan Report with ScanFailed status")
	}
}

func TestGetReportFails(t *testing.T) {

	scanner := &mockImageScanner{
		ExpectedImageDigest: "sha256:somedigest",
		StartScanReturn:     StartScanReturn{Digest: "sha256:somedigest", Error: nil},
		GetReportReturn:     GetReportReturn{Report: nil, Error: fmt.Errorf("Some error")}}
	opaEvaluator := &mockOPAEvaluator{T: t}

	evaluator := NewImageScannerEvaluator(scanner, opaEvaluator, mockGetOPARules, mockGetOPAData)

	a := loadAdmissionRequest("./assets/admission-review.json", t)

	accepted, digestMappings, err := evaluator.ScanAndEvaluate(a, pod)
	if !accepted {
		t.Error(err)
	}

	if digestMappings["mysaferegistry.io/container-image:1.01"] != "sha256:somedigest" {
		t.Fatalf("Unexpected digest mapping: %v", digestMappings)
	}

	if !scanner.StartScanCalled {
		t.Fatalf("StartScan was not called")
	}

	if !scanner.GetReportCalled {
		t.Fatalf("GetReportCalled was not called")
	}

	if !opaEvaluator.Called {
		t.Fatalf("OPAEvaluator.Evaluate was NOT called")
	}

	input := opaEvaluator.ReceivedInput.(OPAInput)
	if input.ScanReport.Status != imagescanner.StatusReportNotAvailable {
		t.Fatalf("OPAEvaluator.Evaluate did not receive a Scan Report with StatusReportNotAvailable status")
	}
}

func TestEvaluationRejects(t *testing.T) {

	report := &imagescanner.ScanReport{
		Status: imagescanner.StatusAccepted,
	}

	scanner := &mockImageScanner{
		T:                   t,
		ExpectedImageAndTag: "mysaferegistry.io/container-image:1.01",
		ExpectedImageDigest: "TestDigest",
		StartScanReturn:     StartScanReturn{Digest: "TestDigest", Error: nil},
		GetReportReturn:     GetReportReturn{Report: report, Error: nil}}

	opaEvaluator := &mockOPAEvaluator{
		T:             t,
		ExpectedQuery: "data.imageadmission.deny_image",
		ExpectedRules: mockRules,
		ExpectedData:  mockData,
		ReturnError:   fmt.Errorf("Reject this container"),
	}

	evaluator := NewImageScannerEvaluator(scanner, opaEvaluator, mockGetOPARules, mockGetOPAData)

	a := loadAdmissionRequest("./assets/admission-review.json", t)

	accepted, _, err := evaluator.ScanAndEvaluate(a, pod)
	if accepted || len(err) != 1 || !strings.Contains(err[0], "Reject this container") {
		t.Errorf("Unexpected evaluation error:\n%v", err)
	}

}

func loadAdmissionRequest(path string, t *testing.T) *v1beta1.AdmissionRequest {
	a := &v1beta1.AdmissionRequest{}
	if b, err := ioutil.ReadFile(path); err != nil {
		t.Fatal(err)
	} else {
		json.Unmarshal(b, a)
	}

	return a
}
