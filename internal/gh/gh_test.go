package gh

import (
	"reflect"
	"testing"
)

func TestParseWorkflowInputs(t *testing.T) {
	src := []byte(`
name: Deploy
on:
  push:
  workflow_dispatch:
    inputs:
      environment:
        description: Target environment
        type: choice
        required: true
        default: staging
        options: [staging, production]
      dry_run:
        type: boolean
        default: false
      note:
        description: Free text
      retries:
        type: number
        default: 3
jobs: {}
`)
	got, err := ParseWorkflowInputs(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []WorkflowInput{
		{Name: "environment", Description: "Target environment", Type: "choice", Default: "staging", Required: true, Options: []string{"staging", "production"}},
		{Name: "dry_run", Type: "boolean", Default: "false"},
		{Name: "note", Description: "Free text", Type: "string"},
		{Name: "retries", Type: "number", Default: "3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestParseWorkflowInputsNone(t *testing.T) {
	for name, src := range map[string]string{
		"bare trigger":  "on:\n  workflow_dispatch:\njobs: {}\n",
		"list form":     "on: [push, workflow_dispatch]\n",
		"string form":   "on: workflow_dispatch\n",
		"no dispatch":   "on:\n  push:\n",
		"empty inputs":  "on:\n  workflow_dispatch:\n    inputs: {}\n",
		"quoted on key": "\"on\":\n  workflow_dispatch:\n",
	} {
		got, err := ParseWorkflowInputs([]byte(src))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(got) != 0 {
			t.Errorf("%s: expected no inputs, got %+v", name, got)
		}
	}
}
