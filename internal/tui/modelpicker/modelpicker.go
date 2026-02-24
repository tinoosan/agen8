package modelpicker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/protocol"
)

type Provider struct {
	Name  string
	Count int
}

type Model struct {
	ID        string
	Provider  string
	InputPerM float64
	OutputPerM float64
}

type Result struct {
	Providers []Provider
	Models    []Model
}

func Fetch(ctx context.Context, endpoint, sessionID, provider, query string) (Result, error) {
	cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}
	var res protocol.ModelListResult
	if err := cli.Call(ctx, protocol.MethodModelList, protocol.ModelListParams{
		ThreadID: protocol.ThreadID(strings.TrimSpace(sessionID)),
		Provider: strings.TrimSpace(provider),
		Query:    strings.TrimSpace(query),
	}, &res); err != nil {
		return Result{}, err
	}
	out := Result{
		Providers: make([]Provider, 0, len(res.Providers)),
		Models:    make([]Model, 0, len(res.Models)),
	}
	for _, p := range res.Providers {
		out.Providers = append(out.Providers, Provider{Name: strings.TrimSpace(p.Name), Count: p.Count})
	}
	for _, m := range res.Models {
		out.Models = append(out.Models, Model{
			ID:         strings.TrimSpace(m.ID),
			Provider:   strings.TrimSpace(m.Provider),
			InputPerM:  m.InputPerM,
			OutputPerM: m.OutputPerM,
		})
	}
	return out, nil
}

func FormatPricePerM(v float64) string {
	if v < 0 {
		v = 0
	}
	return fmt.Sprintf("%.2f", v)
}

func FormatModelTitle(id string, inputPerM, outputPerM float64) string {
	return strings.TrimSpace(id) +
		"  ·  in $" + FormatPricePerM(inputPerM) + "/M" +
		"  ·  out $" + FormatPricePerM(outputPerM) + "/M"
}
