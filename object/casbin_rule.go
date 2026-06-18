package object

import "strings"

type CasbinRule struct {
	Id    int64  `xorm:"pk autoincr" json:"id"`
	PType string `xorm:"varchar(32) notnull" json:"pType"`
	V0    string `xorm:"varchar(256) notnull" json:"v0"`
	V1    string `xorm:"varchar(256)" json:"v1"`
	V2    string `xorm:"varchar(256)" json:"v2"`
	V3    string `xorm:"varchar(256)" json:"v3"`
	V4    string `xorm:"varchar(32)" json:"v4"` // eft: "allow" or "deny" (p-type only)
}

func GetCasbinRules() ([]*CasbinRule, error) {
	var rules []*CasbinRule
	err := ormer.Engine.Find(&rules)
	return rules, err
}

func AddCasbinRule(rule *CasbinRule) error {
	_, err := ormer.Engine.Insert(rule)
	return err
}

func DeleteCasbinRule(id int64) error {
	_, err := ormer.Engine.ID(id).Delete(&CasbinRule{})
	return err
}

// SeedDefaultPolicies inserts an allow-all rule when the table is empty so
// the admission webhook is a no-op out of the box.
func SeedDefaultPolicies() error {
	count, err := ormer.Engine.Count(&CasbinRule{})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = ormer.Engine.Insert(&CasbinRule{
		PType: "p",
		V0:    "*",
		V1:    "*",
		V2:    "*",
		V3:    "*",
		V4:    "allow",
	})
	return err
}

// policiesToText serialises all rules to Casbin CSV format for the enforcer.
func policiesToText(rules []*CasbinRule) string {
	var sb strings.Builder
	for _, r := range rules {
		parts := []string{r.PType, r.V0}
		for _, v := range []string{r.V1, r.V2, r.V3} {
			if v == "" {
				break
			}
			parts = append(parts, v)
		}
		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteString("\n")
	}
	return sb.String()
}
