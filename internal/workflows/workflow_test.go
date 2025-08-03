package workflows

import (
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/parser"
)

func TestExpressionToBooleanConversion(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		context    map[string]interface{}
		expected   bool
	}{
		{
			name:       "Simple equality true",
			expression: "${.orderType == \"electronic\"}",
			context:    map[string]interface{}{"orderType": "electronic"},
			expected:   true,
		},
		{
			name:       "Simple equality false",
			expression: "${.orderType == \"electronic\"}",
			context:    map[string]interface{}{"orderType": "physical"},
			expected:   false,
		},
		{
			name:       "Property access truthy",
			expression: "${.status}",
			context:    map[string]interface{}{"status": "active"},
			expected:   true,
		},
		{
			name:       "Property access falsy",
			expression: "${.status}",
			context:    map[string]interface{}{"status": ""},
			expected:   false,
		},
		{
			name:       "Property access with dot notation",
			expression: ".orderType",
			context:    map[string]interface{}{"orderType": "electronic"},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the underlying expression evaluation
			result, err := evaluateSimpleExpression(tt.expression, tt.context)
			if err != nil {
				t.Fatalf("Expression evaluation failed: %v", err)
			}

			// Convert to boolean using isTruthy
			boolResult := isTruthy(result)
			if boolResult != tt.expected {
				t.Errorf("Expected %v, got %v for expression %s", tt.expected, boolResult, tt.expression)
			}
		})
	}
}

func TestSimpleExpressionEvaluation(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		context    map[string]interface{}
		expected   interface{}
		expectErr  bool
	}{
		{
			name:       "Property access",
			expression: ".orderType",
			context:    map[string]interface{}{"orderType": "electronic"},
			expected:   "electronic",
			expectErr:  false,
		},
		{
			name:       "Equality comparison true",
			expression: ".orderType == \"electronic\"",
			context:    map[string]interface{}{"orderType": "electronic"},
			expected:   true,
			expectErr:  false,
		},
		{
			name:       "Equality comparison false",
			expression: ".orderType == \"physical\"",
			context:    map[string]interface{}{"orderType": "electronic"},
			expected:   false,
			expectErr:  false,
		},
		{
			name:       "Property not found",
			expression: ".nonexistent",
			context:    map[string]interface{}{"orderType": "electronic"},
			expected:   nil,
			expectErr:  true,
		},
		{
			name:       "Direct property name",
			expression: "orderType",
			context:    map[string]interface{}{"orderType": "electronic"},
			expected:   "electronic",
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateSimpleExpression(tt.expression, tt.context)

			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectErr && result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"nil", nil, false},
		{"true", true, true},
		{"false", false, false},
		{"empty string", "", false},
		{"non-empty string", "hello", true},
		{"zero int", 0, false},
		{"positive int", 42, true},
		{"negative int", -1, true},
		{"zero float", 0.0, false},
		{"positive float", 3.14, true},
		{"empty slice", []interface{}{}, false},
		{"non-empty slice", []interface{}{1, 2, 3}, true},
		{"empty map", map[string]interface{}{}, false},
		{"non-empty map", map[string]interface{}{"key": "value"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTruthy(tt.value)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for value %v", tt.expected, result, tt.value)
			}
		})
	}
}

func TestWorkflowParsing(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		shouldPass bool
	}{
		{
			name: "Valid switch workflow",
			yaml: `
document:
  dsl: 1.0.0
  namespace: test
  name: valid-switch
  version: 1.0.0
do:
  - checkCondition:
      switch:
        - case1:
            when: "${.value == \"test\"}"
            then: "continue"
        - default:
            then: "continue"
`,
			shouldPass: true,
		},
		{
			name: "Valid for workflow",
			yaml: `
document:
  dsl: 1.0.0
  namespace: test
  name: valid-for
  version: 1.0.0
do:
  - iterateItems:
      for:
        each: item
        in: "${.items}"
      do:
        - processItem:
            set:
              processed: true
`,
			shouldPass: true,
		},
		{
			name: "Invalid workflow",
			yaml: `
document:
  dsl: 1.0.0
  name: invalid-workflow
do:
  - invalidTask:
      invalidProperty: "test"
`,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.FromYAMLSource([]byte(tt.yaml))
			
			if tt.shouldPass && err != nil {
				t.Errorf("Expected workflow to be valid, but got error: %v", err)
			}
			
			if !tt.shouldPass && err == nil {
				t.Error("Expected workflow to be invalid, but it passed validation")
			}
		})
	}
}