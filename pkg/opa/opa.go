package opa

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/storage/inmem"
	"k8s.io/klog"
)

func (o *opaEvaluator) Evaluate(query, rules, data string, input interface{}) error {

	rs, err := evaluateRules(query, rules, data, input)
	if err != nil {
		return fmt.Errorf("Rego evaluation error: %v", err)
	}

	return evaluateResults(rs)
}

func evaluateRules(query, rules, data string, input interface{}) (rego.ResultSet, error) {

	store, err := parseData(data)
	if err != nil {
		return nil, fmt.Errorf("Error parsing data:\n%v", err)
	}

	compiler, err := compilleRules(rules)
	if err != nil {
		return nil, fmt.Errorf("Rule compilation error:\n%v", err)
	}

	b, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("Error marshalling input to JSON: %v", err)
	}

	klog.V(3).Infof("[rego] Input is:\n%s", string(b))

	ctx := context.Background()
	rego := rego.New(
		rego.Query(query),
		rego.Compiler(compiler),
		rego.Input(input),
		rego.Store(store),
	)

	// Run evaluation.
	return rego.Eval(ctx)
}

func parseData(data string) (storage.Store, error) {
	klog.V(3).Infof("[rego] Data is:\n%s", data)
	var jsonData map[string]interface{}

	err := json.Unmarshal([]byte(data), &jsonData)
	if err != nil {
		return nil, err
	}

	store := inmem.NewFromObject(jsonData)
	return store, nil
}

func compilleRules(rules string) (*ast.Compiler, error) {
	klog.V(3).Infof("[rego] Rules are:\n%s", rules)

	// Compile the module. The keys are used as identifiers in error messages.
	return ast.CompileModules(map[string]string{
		"rules.rego": rules,
	})
}

func evaluateResults(rs rego.ResultSet) error {
	// Inspect results.
	klog.V(3).Infof("[rego] Evaluating ResultSet - len: %d", len(rs))

	if len(rs) != 1 {
		return fmt.Errorf("Rego - unexpected ResultSet length: %d", len(rs))
	}

	klog.V(3).Infof("[rego] Evaluating ResultSet - Expressions length: %d", len(rs[0].Expressions))

	if len(rs[0].Expressions) != 1 {
		return fmt.Errorf("Rego - unexpected Expressions length: %d", len(rs[0].Expressions))
	}

	klog.V(3).Infof("[rego] Evaluating ResultSet - Expressions[0].Value:\n%#v", rs[0].Expressions[0].Value)

	// TODO: Some tests and how to check if it is returning an empty list (OK) or not
	if reflect.ValueOf(rs[0].Expressions[0].Value).Kind() == reflect.Slice {
		if reflect.ValueOf(rs[0].Expressions[0].Value).Len() > 0 {
			values := rs[0].Expressions[0].Value.([]interface{})
			var msgBuilder strings.Builder
			for _, value := range values {
				msgBuilder.WriteString(fmt.Sprintf("- %s\n", value))
			}
			msgList := msgBuilder.String()
			return fmt.Errorf("Image denied by OPA rules:\n%s", msgList)
		}
	}

	return nil
}
