package e2etest

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"

	"github.com/cherryservers/cherrygo/v3"
	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
)

const (
	apiTokenVar       = "CHERRY_TEST_API_TOKEN"
	teamIDVar         = "CHERRY_TEST_TEAM_ID"
	imagePathVar      = "CCM_IMG_PATH"
	noCleanupVar      = "NO_CLEANUP"
	silenceKlogVar    = "SILENCE_KLOG"
	serverPlanVar     = "SERVER_PLAN"
	regionVar         = "REGION"
	k8sVersionVar     = "K8S_VERSION"
	metalLBVersionVar = "METALLB_VERSION"
	kubeVipVersionVar = "KUBE_VIP_VERSION"
)

var (
	cherryClient   *cherrygo.Client
	teamID         *int
	ccmImagePath   *string
	cleanup        *bool
	serverPlan     *string
	region         *string
	k8sVersion     *string
	metalLBVersion *string
	kubeVipVersion *string
)

type config struct {
	apiToken       string
	teamID         int
	ccmImagePath   string
	cleanup        bool
	silenceKlog    bool
	serverPlan     string
	region         string
	k8sVersion     string
	metalLBVersion string
	kubeVipVersion string
}

func getPlanStock(p cherrygo.Plan, region string) int {
	for _, r := range p.AvailableRegions {
		if r.Slug == region {
			return r.StockQty
		}
	}
	return 0
}

func getPlanPrice(p cherrygo.Plan, cycle string) (float32, error) {
	for _, pricing := range p.Pricing {
		if pricing.Unit == cycle {
			return pricing.Price, nil
		}
	}
	return 0, fmt.Errorf("plan doesn't have a price for %q billing cycle", cycle)
}

type planConstraint func(cherrygo.Plan) bool

func planTypeConstraint(t string) planConstraint {
	return func(p cherrygo.Plan) bool {
		return p.Type == t
	}
}

func planStockConstraint(region string, minStock int) planConstraint {
	return func(p cherrygo.Plan) bool {
		return getPlanStock(p, region) >= minStock
	}
}

func planFitsConstraints(p cherrygo.Plan, con ...planConstraint) bool {
	for _, c := range con {
		if !c(p) {
			return false
		}
	}
	return true
}

func getCheapestPlan(plans []cherrygo.Plan, cycle string, con ...planConstraint) (cherrygo.Plan, error) {
	var (
		minIdx int = -1
		minPrice float32 = 0.0
	)

	for i := range plans {
		if !planFitsConstraints(plans[i], con...) {
			continue
		}

		price, err := getPlanPrice(plans[i], cycle)
		if err != nil {
			continue
		}

		if price < minPrice || minIdx < 0 {
			minIdx = i
			minPrice = price
		}
	}

	if minIdx < 0{
		return cherrygo.Plan{}, fmt.Errorf("no viable plan found")
	}
	return plans[minIdx], nil
}

// loadConfig loads test configuration from environment variables.
func loadConfig() (config, error) {
	const (
		defaultK8sVersion     = "1.34"
		defaultMetalLBVersion = "0.15.2"
		defaultKubeVipVersion = "1.0.1"
		defaultRegion         = "LT-Siauliai"
	)

	teamID, err := strconv.Atoi(os.Getenv(teamIDVar))
	if err != nil {
		return config{}, fmt.Errorf("failed to parse team ID: %w", err)
	}

	noCleanup := false
	if noCleanupEnv, ok := os.LookupEnv(noCleanupVar); ok {
		noCleanup, err = strconv.ParseBool(noCleanupEnv)
		if err != nil {
			return config{}, fmt.Errorf("failed to parse %s var: %w", noCleanupVar, err)
		}
	}

	silenceKlog := true
	if silenceKlogEnv, ok := os.LookupEnv(silenceKlogVar); ok {
		silenceKlog, err = strconv.ParseBool(silenceKlogEnv)
		if err != nil {
			return config{}, fmt.Errorf("failed to parse %s var: %w", silenceKlogVar, err)
		}
	}

	serverPlan := os.Getenv(serverPlanVar)

	region := defaultRegion
	if regionEnv, ok := os.LookupEnv(regionVar); ok {
		region = regionEnv
	}

	k8sVersion := defaultK8sVersion
	if k8sVersionEnv, ok := os.LookupEnv(k8sVersionVar); ok {
		k8sVersion = k8sVersionEnv
	}

	metalLBVersion := defaultMetalLBVersion
	if metalLBVersionEnv, ok := os.LookupEnv(metalLBVersionVar); ok {
		metalLBVersion = metalLBVersionEnv
	}

	kubeVipVersion := defaultKubeVipVersion
	if kubeVipVersionEnv, ok := os.LookupEnv(kubeVipVersionVar); ok {
		kubeVipVersion = kubeVipVersionEnv
	}

	return config{
		apiToken:       os.Getenv(apiTokenVar),
		teamID:         teamID,
		ccmImagePath:   os.Getenv(imagePathVar),
		cleanup:        !noCleanup,
		silenceKlog:    silenceKlog,
		serverPlan:     serverPlan,
		region:         region,
		k8sVersion:     k8sVersion,
		metalLBVersion: metalLBVersion,
		kubeVipVersion: kubeVipVersion,
	}, nil
}

// get cheapest server plan with vds type and ok stock
func getDefaultServerPlan() (string, error) {
	const (
		planMinStock     = 15
		planType         = "vds"
		planBillingCycle = "Hourly"
	)

	plans, _, err := cherryClient.Plans.List(*teamID, nil)
	if err != nil {
		return "", err
	}

	plan, err := getCheapestPlan(plans, planBillingCycle,
		planTypeConstraint(planType), planStockConstraint(*region, planMinStock))
	if err != nil {
		return "", err
	}

	return plan.Slug, nil
}

func runMain(m *testing.M) int {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load test config: %v", err)
	}

	cherryClient, err = cherrygo.NewClient(cherrygo.WithAuthToken(cfg.apiToken))
	if err != nil {
		log.Fatalf("failed to initialize cherrygo client: %v", err)
	}

	teamID = &cfg.teamID
	ccmImagePath = &cfg.ccmImagePath
	cleanup = &cfg.cleanup
	serverPlan = &cfg.serverPlan
	region = &cfg.region
	k8sVersion = &cfg.k8sVersion
	metalLBVersion = &cfg.metalLBVersion
	kubeVipVersion = &cfg.kubeVipVersion

	if *serverPlan == "" {
		*serverPlan, err = getDefaultServerPlan()
		if err != nil {
			log.Fatalf("failed to set default server plan: %v", err)
		}
	}

	if cfg.silenceKlog {
		klog.SetLogger(logr.Discard())
	}

	code := m.Run()
	return code
}

func TestMain(m *testing.M) {
	code := runMain(m)
	os.Exit(code)
}
