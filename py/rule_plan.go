package py

import (
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

type rulePlan struct {
	rule    *rule.Rule
	imports ImportData
}

func generateResultFromPlans(plans []rulePlan, cfg *pyConfig) language.GenerateResult {
	if len(plans) == 0 {
		return language.GenerateResult{}
	}

	genRules := make([]*rule.Rule, 0, len(plans))
	genImports := make([]interface{}, 0, len(plans))
	for _, plan := range plans {
		genRules = append(genRules, plan.rule)
		genImports = append(genImports, withConfigSnapshot(plan.imports, cfg))
	}
	return language.GenerateResult{
		Gen:     genRules,
		Imports: genImports,
	}
}

func splitRulePlans(plans []rulePlan, cfg *pyConfig) ([]*rule.Rule, []interface{}) {
	genRules := make([]*rule.Rule, 0, len(plans))
	genImports := make([]interface{}, 0, len(plans))
	for _, plan := range plans {
		genRules = append(genRules, plan.rule)
		genImports = append(genImports, withConfigSnapshot(plan.imports, cfg))
	}
	return genRules, genImports
}

func withConfigSnapshot(data ImportData, cfg *pyConfig) ImportData {
	if cfg != nil {
		data.config = cfg.clone()
	}
	return data
}
