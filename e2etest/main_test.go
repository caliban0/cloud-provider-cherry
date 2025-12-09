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
	k8sVersion     string
	metalLBVersion string
	kubeVipVersion string
}

// loadConfig loads test configuration from environment variables.
func loadConfig() (config, error) {
	const (
		defaultK8sVersion     = "1.34"
		defaultMetalLBVersion = "0.15.2"
		defaultKubeVipVersion = "1.0.1"
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

	serverPlan := "G1-8-32gb-200nv-ded"
	if serverPlanEnv, ok := os.LookupEnv(serverPlanVar); ok {
		serverPlan = serverPlanEnv
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
		k8sVersion:     k8sVersion,
		metalLBVersion: metalLBVersion,
		kubeVipVersion: kubeVipVersion,
	}, nil
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
	k8sVersion = &cfg.k8sVersion
	metalLBVersion = &cfg.metalLBVersion
	kubeVipVersion = &cfg.kubeVipVersion

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
