package test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/entigolabs/function-base/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
)

func IgnoreFields(fields ...string) cmp.Option {
	fieldsMap := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		fieldsMap[fmt.Sprintf(`["%s"]`, field)] = nil
	}
	return cmp.FilterPath(func(p cmp.Path) bool {
		vx := p.Last().String()
		_, ignored := fieldsMap[vx]
		return ignored
	}, cmp.Ignore())
}

type Args struct {
	Ctx context.Context
	Req *fnv1.RunFunctionRequest
}

type Want struct {
	Rsp *fnv1.RunFunctionResponse
	Err error
}

type Case struct {
	Reason string
	Args   Args
	Want   Want
}

func RunFunctionCases(t *testing.T, serviceFn func() base.GroupService, cases map[string]Case, ignoredFields ...string) {
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := base.NewFunction(logging.NewNopLogger(), serviceFn())
			rsp, err := f.RunFunction(tc.Args.Ctx, tc.Args.Req)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if len(rsp.GetResults()) > 0 && rsp.GetResults()[0].GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
				t.Errorf("Response failure: %v", rsp.GetResults()[0].GetMessage())
				return
			}
			if diff := cmp.Diff(tc.Want.Rsp, rsp, protocmp.Transform(), IgnoreFields(ignoredFields...)); diff != "" {
				//Can be used to print the desired resources
				for key, value := range rsp.GetDesired().GetResources() {
					fmt.Println(key)
					rspResource, _ := json.Marshal(value.Resource)
					fmt.Println(string(rspResource))
				}
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.Reason, diff)
			}

			if diff := cmp.Diff(tc.Want.Err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.Reason, diff)
			}
		})
	}
}
