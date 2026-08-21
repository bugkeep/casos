package store

import (
	"reflect"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func testChart(name string) *chart.Chart {
	return &chart.Chart{
		Metadata: &chart.Metadata{Name: name},
		Values:   map[string]interface{}{},
	}
}

func TestHelmChartAdapterInjectsNodePort(t *testing.T) {
	for _, app := range []string{"grafana", "pgadmin4", "n8n", "superset", "nextcloud"} {
		values, adjustments, err := prepareHelmInstallValues(testChart(app), "https://example.com/charts", map[string]interface{}{})
		if err != nil {
			t.Fatalf("%s: %v", app, err)
		}
		service, ok := values["service"].(map[string]interface{})
		if !ok || service["type"] != "NodePort" {
			t.Errorf("%s: expected service.type NodePort, got %#v", app, values["service"])
		}
		if adjustments.legacyImages || adjustments.tomcatDefaultWebapps {
			t.Errorf("%s: non-Bitnami chart should not apply Bitnami adjustments", app)
		}
	}
}

func TestHelmChartAdapterRespectsExplicitServiceType(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("n8n"), "https://example.com/charts", map[string]interface{}{
		"service": map[string]interface{}{"type": "LoadBalancer"},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "LoadBalancer" {
		t.Errorf("expected user service.type LoadBalancer preserved, got %#v", values["service"])
	}
}

func TestMetricsServerAdapterAcceptsLocalKubeletCertificate(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("metrics-server"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	want := []interface{}{"--kubelet-insecure-tls"}
	if got := values["args"]; !reflect.DeepEqual(got, want) {
		t.Errorf("expected kubelet TLS compatibility arg, got %#v", got)
	}
}

func TestSecondPageWebAppsArePublished(t *testing.T) {
	tests := []struct {
		name string
		path []string
	}{
		{"vault", []string{"server", "service", "type"}},
		{"keycloak", []string{"service", "type"}},
		{"jenkins", []string{"controller", "serviceType"}},
		{"longhorn", []string{"service", "ui", "type"}},
		{"rabbitmq", []string{"service", "type"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			values, _, err := prepareHelmInstallValues(testChart(testCase.name), "https://example.com/charts", map[string]interface{}{})
			if err != nil {
				t.Fatalf("prepare values: %v", err)
			}
			if got := helmValueAtPath(values, testCase.path); got != "NodePort" {
				t.Errorf("expected %v to be NodePort, got %#v", testCase.path, got)
			}
		})
	}
}

func TestHarborAdapterPublishesPlainHTTPWithAllocatedPort(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("harbor"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if got := helmValueAtPath(values, []string{"expose", "type"}); got != "nodePort" {
		t.Errorf("expected expose.type nodePort, got %#v", got)
	}
	if got := helmValueAtPath(values, []string{"expose", "tls", "enabled"}); got != false {
		t.Errorf("expected plain HTTP exposure, got %#v", got)
	}
	httpValues := helmValueMapAtPath(values, "expose", "nodePort", "ports", "http")
	httpNodePort, exists := httpValues["nodePort"]
	if !exists || httpNodePort != nil {
		t.Errorf("expected Kubernetes-allocated HTTP NodePort, got %#v (exists=%v)", httpNodePort, exists)
	}
}

func TestGitLabAdapterDoesNotRequireACMEIdentity(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("gitlab"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	for _, path := range [][]string{{"installCertmanager"}, {"global", "gatewayApi", "configureCertmanager"}, {"global", "ingress", "configureCertmanager"}} {
		if got := helmValueAtPath(values, path); got != false {
			t.Errorf("expected %v false, got %#v", path, got)
		}
	}
}

func TestHelmChartAdapterSkipsUnregisteredChart(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("some-other-app"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if _, exists := values["service"]; exists {
		t.Errorf("unregistered chart should not be patched, got %#v", values["service"])
	}
}

func TestHelmChartAdapterKeepsBitnamiAdjustments(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{Name: "grafana"},
		Values: map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "bitnami/grafana",
				"tag":        "11.6.1-debian-12-r0",
			},
		},
	}
	values, adjustments, err := prepareHelmInstallValues(ch, bitnamiChartRepoURL, map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if !adjustments.legacyImages {
		t.Error("expected Bitnami legacy image adjustment to be preserved")
	}
	image, ok := values["image"].(map[string]interface{})
	if !ok || image["repository"] != "bitnamilegacy/grafana" {
		t.Errorf("expected rewritten legacy image repository, got %#v", values["image"])
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("grafana should get NodePort even on Bitnami repo, got %#v", values["service"])
	}
}

func TestHelmChartAdapterOverridesModeInjectNodePortWhenSiblingChanged(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(testChart("grafana"), "https://example.com/charts", map[string]interface{}{
		"service": map[string]interface{}{"port": 3001},
	}, helmInstallValueOptions{inputIsOverrides: true})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, _ := values["service"].(map[string]interface{})
	if service["type"] != "NodePort" {
		t.Errorf("adapter must inject NodePort when only a sibling key changed; got %#v", values["service"])
	}
	if service["port"] != 3001 {
		t.Errorf("user port must be preserved, got %#v", values["service"])
	}
}

func TestHelmChartAdapterOverridesModeRespectsExplicitType(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(testChart("grafana"), "https://example.com/charts", map[string]interface{}{
		"service": map[string]interface{}{"type": "LoadBalancer", "port": 3001},
	}, helmInstallValueOptions{inputIsOverrides: true})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, _ := values["service"].(map[string]interface{})
	if service["type"] != "LoadBalancer" {
		t.Errorf("user service.type must win; got %#v", values["service"])
	}
}

func supersetSecretKey(t *testing.T, values map[string]interface{}) string {
	t.Helper()
	env, ok := values["extraSecretEnv"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extraSecretEnv, got %#v", values["extraSecretEnv"])
	}
	key, ok := env["SUPERSET_SECRET_KEY"].(string)
	if !ok {
		t.Fatalf("expected SUPERSET_SECRET_KEY string, got %#v", env["SUPERSET_SECRET_KEY"])
	}
	return key
}

func TestSupersetAdapterPatches(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("expected service.type NodePort, got %#v", values["service"])
	}
	nodePort, ok := service["nodePort"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected service.nodePort map, got %#v", service["nodePort"])
	}
	// A real null, not the chart's "nil" string and not a fixed port number.
	if value, exists := nodePort["http"]; !exists || value != nil {
		t.Errorf("expected service.nodePort.http to be null, got %#v", nodePort["http"])
	}
	if key := supersetSecretKey(t, values); len(key) < 32 {
		t.Errorf("expected a long random SECRET_KEY, got %q", key)
	}
	if _, exists := values["configOverrides"]; exists {
		t.Errorf("SECRET_KEY must not land in configOverrides, got %#v", values["configOverrides"])
	}
	bootstrap, _ := values["bootstrapScript"].(string)
	if !strings.Contains(bootstrap, "psycopg2-binary=="+supersetPsycopg2Version) {
		t.Errorf("expected a pinned psycopg2-binary in bootstrapScript, got %q", bootstrap)
	}
	if !strings.Contains(bootstrap, "import psycopg2") {
		t.Errorf("expected bootstrapScript to skip the install when the driver exists, got %q", bootstrap)
	}
	if strings.Contains(bootstrap, "export PYTHONPATH="+supersetDriverTarget+":") {
		t.Errorf("PYTHONPATH must not end in a bare separator, got %q", bootstrap)
	}
}

func TestSupersetAdapterGeneratesDistinctSecretKeys(t *testing.T) {
	first, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	second, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if supersetSecretKey(t, first) == supersetSecretKey(t, second) {
		t.Error("each install must get its own SECRET_KEY")
	}
}

func TestSupersetAdapterRespectsUserSecret(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "user-secret"},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if key := supersetSecretKey(t, values); key != "user-secret" {
		t.Errorf("user SECRET_KEY should win, got %q", key)
	}
}

func TestSupersetAdapterSkipsSecretWhenConfigOverrideSetsIt(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{
		"configOverrides": map[string]interface{}{"secret": `SECRET_KEY = "legacy"`},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if _, exists := values["extraSecretEnv"]; exists {
		t.Errorf("no second SECRET_KEY should be generated, got %#v", values["extraSecretEnv"])
	}
}

func TestSupersetValuesPreviewIsStableAndSecretFree(t *testing.T) {
	first, _, err := buildHelmChartInstallValues(testChart("superset"), "https://example.com/charts")
	if err != nil {
		t.Fatalf("build values: %v", err)
	}
	second, _, err := buildHelmChartInstallValues(testChart("superset"), "https://example.com/charts")
	if err != nil {
		t.Fatalf("build values: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("values preview must be reproducible; got %#v then %#v", first, second)
	}
	if _, exists := first["extraSecretEnv"]; exists {
		t.Errorf("values preview must not mint a SECRET_KEY, got %#v", first["extraSecretEnv"])
	}
}

func seedSupersetRelease(t *testing.T, config map[string]interface{}) *action.Configuration {
	t.Helper()
	cfg := &action.Configuration{}
	mem := driver.NewMemory()
	cfg.Releases = storage.Init(mem)
	if err := mem.Create("superset-demo", &release.Release{
		Name:   "superset-demo",
		Chart:  testChart("superset"),
		Info:   &release.Info{Status: release.StatusDeployed},
		Config: config,
	}); err != nil {
		t.Fatalf("seed release: %v", err)
	}
	return cfg
}

func TestPreserveSupersetSecretKeyOnUpgrade(t *testing.T) {
	cfg := seedSupersetRelease(t, map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "installed-key"},
	})
	ch := testChart("superset")

	vals := map[string]interface{}{}
	preserveHelmChartAdapterValues(cfg, ch, "superset-demo", vals)
	if key := supersetSecretKey(t, vals); key != "installed-key" {
		t.Fatalf("expected the installed SECRET_KEY to be reused, got %q", key)
	}

	userVals := map[string]interface{}{"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "user-key"}}
	preserveHelmChartAdapterValues(cfg, ch, "superset-demo", userVals)
	if key := supersetSecretKey(t, userVals); key != "user-key" {
		t.Fatalf("user-provided SECRET_KEY must win, got %q", key)
	}
}

func TestPreserveLegacySupersetSecretKeyOnUpgrade(t *testing.T) {
	cfg := seedSupersetRelease(t, map[string]interface{}{
		"configOverrides": map[string]interface{}{"secret": `SECRET_KEY = "legacy-key"`},
	})

	vals := map[string]interface{}{}
	preserveHelmChartAdapterValues(cfg, testChart("superset"), "superset-demo", vals)
	overrides, _ := vals["configOverrides"].(map[string]interface{})
	if overrides["secret"] != `SECRET_KEY = "legacy-key"` {
		t.Fatalf("expected the legacy SECRET_KEY to be reused, got %#v", vals["configOverrides"])
	}

	prepared, _, err := prepareHelmInstallValuesWithOptions(testChart("superset"), "https://example.com/charts", vals, helmInstallValueOptions{inputIsOverrides: true})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if _, exists := prepared["extraSecretEnv"]; exists {
		t.Errorf("an upgrade must not rotate the key onto a new mechanism, got %#v", prepared["extraSecretEnv"])
	}
}

func TestPreserveHelmChartAdapterValuesIgnoresOtherCharts(t *testing.T) {
	cfg := seedSupersetRelease(t, map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "installed-key"},
	})
	vals := map[string]interface{}{}
	preserveHelmChartAdapterValues(cfg, testChart("grafana"), "superset-demo", vals)
	if len(vals) != 0 {
		t.Errorf("charts without preserved paths must be left alone, got %#v", vals)
	}
}

// nextcloudChart mirrors the upstream chart's defaults for the values the host
// binding reads: nextcloud.host doubles as the Host header the chart's probes
// send to /status.php.
func nextcloudChart() *chart.Chart {
	ch := testChart("nextcloud")
	ch.Values["nextcloud"] = map[string]interface{}{
		"host":           "nextcloud.kube.home",
		"trustedDomains": []interface{}{},
	}
	ch.Values["httpRoute"] = map[string]interface{}{"hostnames": []interface{}{}}
	return ch
}

func boundHosts(t *testing.T, values map[string]interface{}, path ...string) []string {
	t.Helper()
	hosts := helmValueHosts(values, path)
	if hosts == nil {
		t.Fatalf("no hosts bound at %v, got %#v", path, values)
	}
	return hosts
}

func TestClusterHostBindingPublishesClusterAddresses(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(nextcloudChart(), "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{
		cluster: clusterContext{
			nodeIPs:     func() []string { return []string{"192.168.10.101", "203.0.113.7"} },
			releaseName: "cloud",
			namespace:   "apps",
		},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	hosts := boundHosts(t, values, "nextcloud", "trustedDomains")
	want := []string{
		"nextcloud.kube.home",
		"localhost",
		"cloud.apps.svc.cluster.local",
		"cloud-nextcloud.apps.svc.cluster.local",
		"192.168.10.101",
		"203.0.113.7",
	}
	if !reflect.DeepEqual(hosts, want) {
		t.Errorf("bound hosts = %#v, want %#v", hosts, want)
	}
}

func TestClusterHostBindingKeepsUserHostsAhead(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(nextcloudChart(), "https://example.com/charts", map[string]interface{}{
		"nextcloud": map[string]interface{}{
			"host":           "cloud.example.com",
			"trustedDomains": []interface{}{"old.example.com", "cloud.example.com"},
		},
		"httpRoute": map[string]interface{}{
			"hostnames": []interface{}{"route.example.com", "old.example.com"},
		},
	}, helmInstallValueOptions{
		cluster: clusterContext{nodeIPs: func() []string { return []string{"192.168.10.101", "192.168.10.101"} }},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	hosts := boundHosts(t, values, "nextcloud", "trustedDomains")
	want := []string{
		"cloud.example.com",
		"nextcloud.kube.home",
		"old.example.com",
		"route.example.com",
		"localhost",
		"192.168.10.101",
	}
	if !reflect.DeepEqual(hosts, want) {
		t.Errorf("bound hosts = %#v, want %#v", hosts, want)
	}
}

func TestClusterHostBindingWithoutClusterContext(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(nextcloudChart(), "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	hosts := boundHosts(t, values, "nextcloud", "trustedDomains")
	if !reflect.DeepEqual(hosts, []string{"nextcloud.kube.home", "localhost"}) {
		t.Errorf("a nil node lookup must still keep the probe host, got %#v", hosts)
	}
}

func TestClusterHostBindingSkippedInInstallPreview(t *testing.T) {
	called := false
	values, _, err := prepareHelmInstallValuesWithOptions(nextcloudChart(), "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{
		skipDynamicValues: true,
		cluster:           clusterContext{nodeIPs: func() []string { called = true; return []string{"192.168.10.101"} }},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if called {
		t.Error("the install preview must not reach the API server for node addresses")
	}
	if _, exists := values["nextcloud"]; exists {
		t.Errorf("the preview must not bind cluster hosts, got %#v", values["nextcloud"])
	}
}

func TestChartsWithoutHostBindingAreLeftAlone(t *testing.T) {
	called := false
	values, _, err := prepareHelmInstallValuesWithOptions(testChart("grafana"), "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{
		cluster: clusterContext{
			nodeIPs:     func() []string { called = true; return []string{"192.168.10.101"} },
			releaseName: "g",
			namespace:   "apps",
		},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if called {
		t.Error("a chart without a host binding must not list the cluster nodes")
	}
	if len(values) != 1 || values["service"] == nil {
		t.Errorf("only the NodePort patch belongs to a chart without a host binding, got %#v", values)
	}
}

func TestHelmValueHosts(t *testing.T) {
	values := map[string]interface{}{
		"single":  "one.example.com",
		"many":    []interface{}{"a.example.com", 42, "b.example.com"},
		"nested":  map[string]interface{}{"leaf": []interface{}{"c.example.com"}},
		"wrong":   map[string]interface{}{"leaf": 7},
		"strings": []string{"d.example.com", "e.example.com"},
	}
	for _, testCase := range []struct {
		path []string
		want []string
	}{
		{[]string{"single"}, []string{"one.example.com"}},
		{[]string{"many"}, []string{"a.example.com", "b.example.com"}},
		{[]string{"strings"}, []string{"d.example.com", "e.example.com"}},
		{[]string{"nested", "leaf"}, []string{"c.example.com"}},
		{[]string{"wrong", "leaf"}, nil},
		{[]string{"missing", "leaf"}, nil},
		{[]string{"single", "leaf"}, nil},
	} {
		if got := helmValueHosts(values, testCase.path); !reflect.DeepEqual(got, testCase.want) {
			t.Errorf("helmValueHosts(%v) = %#v, want %#v", testCase.path, got, testCase.want)
		}
	}
}

func TestHelmValuePatch(t *testing.T) {
	got := helmValuePatch([]string{"a", "b"}, []interface{}{"host"})
	want := map[string]interface{}{"a": map[string]interface{}{"b": []interface{}{"host"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("helmValuePatch = %#v, want %#v", got, want)
	}
	if got := helmValuePatch([]string{"a"}, "x"); !reflect.DeepEqual(got, map[string]interface{}{"a": "x"}) {
		t.Errorf("single-segment patch = %#v", got)
	}
}

func TestClusterHostBindingSkipsRedundantServiceName(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(nextcloudChart(), "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{
		cluster: clusterContext{releaseName: "my-nextcloud", namespace: "apps"},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	hosts := boundHosts(t, values, "nextcloud", "trustedDomains")
	want := []string{"nextcloud.kube.home", "localhost", "my-nextcloud.apps.svc.cluster.local"}
	if !reflect.DeepEqual(hosts, want) {
		t.Errorf("a release name carrying the chart name needs one service host, got %#v", hosts)
	}
}

func TestReleaseServiceNames(t *testing.T) {
	// 62 characters: appending "-nextcloud" and truncating leaves a trailing
	// dash, which trims back to the release name itself.
	longName := strings.Repeat("n", 62)
	for _, testCase := range []struct {
		name        string
		releaseName string
		values      map[string]interface{}
		want        []string
	}{
		{"chart name is appended", "cloud", nil, []string{"cloud", "cloud-nextcloud"}},
		{"release already carries the chart name", "my-nextcloud", nil, []string{"my-nextcloud"}},
		{"fullnameOverride decides", "cloud", map[string]interface{}{"fullnameOverride": "files"}, []string{"cloud", "files"}},
		{"nameOverride replaces the chart name", "cloud", map[string]interface{}{"nameOverride": "files"}, []string{"cloud", "cloud-files"}},
		{"blank override is ignored", "cloud", map[string]interface{}{"fullnameOverride": "  "}, []string{"cloud", "cloud-nextcloud"}},
		{"truncated back to the release name", longName, nil, []string{longName}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cluster := clusterContext{releaseName: testCase.releaseName, namespace: "apps"}
			if got := cluster.releaseServiceNames(nextcloudChart(), testCase.values); !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("releaseServiceNames = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestReleaseServiceNamesPrefersChartDefaultOverride(t *testing.T) {
	ch := nextcloudChart()
	ch.Values["nameOverride"] = "files"
	cluster := clusterContext{releaseName: "cloud", namespace: "apps"}
	if got := cluster.releaseServiceNames(ch, map[string]interface{}{}); !reflect.DeepEqual(got, []string{"cloud", "cloud-files"}) {
		t.Errorf("a chart's own nameOverride must be honoured, got %#v", got)
	}
	if got := cluster.releaseServiceNames(ch, map[string]interface{}{"nameOverride": "photos"}); !reflect.DeepEqual(got, []string{"cloud", "cloud-photos"}) {
		t.Errorf("install values must win over the chart default, got %#v", got)
	}
}

func TestMemoizedNodeIPsResolvesOnce(t *testing.T) {
	calls := 0
	nodeIPs := memoizedNodeIPs(func() []string {
		calls++
		return []string{"192.168.10.101"}
	})
	if calls != 0 {
		t.Fatalf("the node lookup must stay lazy until a binding asks for it, got %d calls", calls)
	}
	first, second := nodeIPs(), nodeIPs()
	if calls != 1 {
		t.Errorf("expected a single node list call, got %d", calls)
	}
	if !reflect.DeepEqual(first, []string{"192.168.10.101"}) || !reflect.DeepEqual(first, second) {
		t.Errorf("memoized lookup returned %#v then %#v", first, second)
	}
}

// n8nCommunityChart mirrors the community-charts layout: a name/value map in
// main.extraEnvVars, with main.extraEnv reserved for valueFrom entries.
func n8nCommunityChart() *chart.Chart {
	return &chart.Chart{
		Metadata: &chart.Metadata{Name: "n8n"},
		Values: map[string]interface{}{
			"extraEnv":     []interface{}{},
			"extraEnvVars": map[string]interface{}{},
			"main": map[string]interface{}{
				"extraEnv":     []interface{}{},
				"extraEnvVars": map[string]interface{}{},
			},
		},
	}
}

// n8nLegacyChart mirrors the 8gears layout: main.extraEnv keyed by variable
// name, holding the body of a container env entry, and no extraEnvVars at all.
func n8nLegacyChart() *chart.Chart {
	return &chart.Chart{
		Metadata: &chart.Metadata{Name: "n8n"},
		Values: map[string]interface{}{
			"main": map[string]interface{}{"extraEnv": nil},
		},
	}
}

func TestN8nAdapterDisablesSecureCookieOnCommunityChart(t *testing.T) {
	values, _, err := prepareHelmInstallValues(n8nCommunityChart(), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	env := helmValueMapAtPath(values, "main", "extraEnvVars")
	if env["N8N_SECURE_COOKIE"] != "false" {
		t.Errorf("expected main.extraEnvVars.N8N_SECURE_COOKIE=false, got %#v", values["main"])
	}
	if _, exists := values["extraEnvVars"]; exists {
		t.Errorf("the deprecated top-level map must be left alone, got %#v", values["extraEnvVars"])
	}
}

func TestN8nAdapterDisablesSecureCookieOnLegacyChart(t *testing.T) {
	values, _, err := prepareHelmInstallValues(n8nLegacyChart(), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	entry := helmValueMapAtPath(values, "main", "extraEnv", "N8N_SECURE_COOKIE")
	if entry["value"] != "false" {
		t.Errorf("expected the env entry keyed by name, got %#v", values["main"])
	}
}

func TestN8nAdapterSkipsChartWithoutEnvValues(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("n8n"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if _, exists := values["main"]; exists {
		t.Errorf("a chart declaring no env values path must not be patched, got %#v", values["main"])
	}
	if service, _ := values["service"].(map[string]interface{}); service["type"] != "NodePort" {
		t.Errorf("the NodePort patch must still apply, got %#v", values["service"])
	}
}

func TestN8nAdapterKeepsUserEnvVars(t *testing.T) {
	values, _, err := prepareHelmInstallValues(n8nCommunityChart(), "https://example.com/charts", map[string]interface{}{
		"main": map[string]interface{}{
			"extraEnvVars": map[string]interface{}{"TZ": "Asia/Shanghai"},
		},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	env := helmValueMapAtPath(values, "main", "extraEnvVars")
	if env["TZ"] != "Asia/Shanghai" || env["N8N_SECURE_COOKIE"] != "false" {
		t.Errorf("user env vars and the adapter default must coexist, got %#v", env)
	}
}

func TestN8nAdapterRespectsExplicitSecureCookie(t *testing.T) {
	values, _, err := prepareHelmInstallValues(n8nCommunityChart(), "https://example.com/charts", map[string]interface{}{
		"main": map[string]interface{}{
			"extraEnvVars": map[string]interface{}{"N8N_SECURE_COOKIE": "true"},
		},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	env := helmValueMapAtPath(values, "main", "extraEnvVars")
	if env["N8N_SECURE_COOKIE"] != "true" {
		t.Errorf("the user's own value must win, got %#v", env)
	}
}

// The chart reads main.extraEnvVars in preference to the top-level map, so
// patching main would silently drop what the user wrote at the top level.
func TestN8nAdapterJoinsTopLevelEnvVars(t *testing.T) {
	values, _, err := prepareHelmInstallValues(n8nCommunityChart(), "https://example.com/charts", map[string]interface{}{
		"extraEnvVars": map[string]interface{}{"TZ": "Asia/Shanghai"},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	env := helmValueMapAtPath(values, "extraEnvVars")
	if env["TZ"] != "Asia/Shanghai" || env["N8N_SECURE_COOKIE"] != "false" {
		t.Errorf("expected both entries in the map the chart will read, got %#v", env)
	}
	if _, exists := helmValueMapAtPath(values, "main")["extraEnvVars"]; exists {
		t.Errorf("patching main would shadow the user's top-level map, got %#v", values["main"])
	}
}

func TestN8nSecureCookieWarningFollowsTheValue(t *testing.T) {
	rendered, err := renderHelmChartInstallValues(n8nCommunityChart(), "https://example.com/charts")
	if err != nil {
		t.Fatalf("render values: %v", err)
	}
	if !strings.Contains(rendered, "# WARNING:") || !strings.Contains(rendered, "N8N_SECURE_COOKIE") {
		t.Errorf("the install dialog must explain the downgrade, got:\n%s", rendered)
	}
	if got := n8nSecureCookieWarning(map[string]interface{}{
		"main": map[string]interface{}{"extraEnvVars": map[string]interface{}{"N8N_SECURE_COOKIE": "true"}},
	}); got != "" {
		t.Errorf("a user who turned it back on must not be warned, got %q", got)
	}
}

func TestOtherAdaptersHaveNoWarning(t *testing.T) {
	for _, app := range []string{"grafana", "pgadmin4", "superset", "nextcloud", "some-other-app"} {
		if got := helmChartAdapterWarning(testChart(app), map[string]interface{}{}); got != "" {
			t.Errorf("%s: unexpected warning %q", app, got)
		}
	}
}
