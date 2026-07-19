package server

import "testing"

func TestValidateApplicationAccessConfig(t *testing.T) {
	for _, test := range []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "both disabled", config: Config{}},
		{name: "service LB only", config: Config{ServiceLBEnabled: true}},
		{name: "complete data plane", config: Config{IngressControllerEnabled: true, ServiceLBEnabled: true}},
		{name: "ingress without service LB", config: Config{IngressControllerEnabled: true}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateApplicationAccessConfig(test.config)
			if (err != nil) != test.wantErr {
				t.Fatalf("validate config error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
