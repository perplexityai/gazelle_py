package py

import (
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

type rulePlan struct {
	rule    *rule.Rule
	imports ImportData
}

func generateResultFromPlans(plans []rulePlan) language.GenerateResult {
	if len(plans) == 0 {
		return language.GenerateResult{}
	}

	genRules := make([]*rule.Rule, 0, len(plans))
	genImports := make([]interface{}, 0, len(plans))
	for _, plan := range plans {
		genRules = append(genRules, plan.rule)
		genImports = append(genImports, plan.imports)
	}
	return language.GenerateResult{
		Gen:     genRules,
		Imports: genImports,
	}
}

func splitRulePlans(plans []rulePlan) ([]*rule.Rule, []interface{}) {
	genRules := make([]*rule.Rule, 0, len(plans))
	genImports := make([]interface{}, 0, len(plans))
	for _, plan := range plans {
		genRules = append(genRules, plan.rule)
		genImports = append(genImports, plan.imports)
	}
	return genRules, genImports
}
