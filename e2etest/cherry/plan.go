package cherry

import (
	"fmt"

	"github.com/cherryservers/cherrygo/v3"
)

type planClient interface {
	List(teamID int, opts *cherrygo.GetOptions) ([]cherrygo.Plan, *cherrygo.Response, error)
}

type PlanClient struct {
	c planClient
}

func newPlanClient(c planClient) PlanClient {
	return PlanClient{c: c}
}

// Pseudo-constant for the plan fields we want to get from the API.
var planGetFields = []string{"slug", "type", "pricing", "available_regions", "stock_qty"}

// GetCheapest returns the slug of the cheapest plan for the billing cycle.
func (c PlanClient) GetCheapest(
	teamID int,
	cycle string,
	con ...PlanConstraint) (slug string, err error) {
	plans, _, err := c.c.List(teamID, &cherrygo.GetOptions{Fields: planGetFields})
	if err != nil {
		return "", fmt.Errorf("couldn't list plans: %w", err)
	}

	p, err := getCheapestPlan(plans, cycle, con...)
	return p.Slug, err
}

func getCheapestPlan(
	plans []cherrygo.Plan,
	cycle string,
	con ...PlanConstraint) (cherrygo.Plan, error) {
	var (
		minIdx   = -1
		minPrice float32
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

	if minIdx < 0 {
		return cherrygo.Plan{}, fmt.Errorf("no viable plan found")
	}
	return plans[minIdx], nil
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

type PlanConstraint func(cherrygo.Plan) bool

// PlanTypeConstraint constrains plans by "type" field.
func PlanTypeConstraint(t string) PlanConstraint {
	return func(p cherrygo.Plan) bool {
		return p.Type == t
	}
}

// PlanStockConstraint constrains plans to have enough stock.
func PlanStockConstraint(region string, minStock int) PlanConstraint {
	return func(p cherrygo.Plan) bool {
		return getPlanStock(p, region) >= minStock
	}
}

func planFitsConstraints(p cherrygo.Plan, con ...PlanConstraint) bool {
	for _, c := range con {
		if !c(p) {
			return false
		}
	}
	return true
}
