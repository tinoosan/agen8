#!/usr/bin/env python3
"""Discounted Cash Flow (DCF) calculator.

Usage:
    python calculate_dcf.py --cash-flows 100 110 121 133 146 --discount-rate 0.10
    python calculate_dcf.py --cash-flows 100 110 121 --growth-rate 0.05 --years 10 --discount-rate 0.10
    python calculate_dcf.py --from-json input.json
    python calculate_dcf.py --cash-flows 50 60 70 --discount-rate 0.08 --terminal-growth 0.02

Input JSON format:
    {
        "cash_flows": [100, 110, 121],
        "discount_rate": 0.10,
        "terminal_growth_rate": 0.02,
        "shares_outstanding": 1000000
    }
"""

import argparse
import json
import sys


def calculate_dcf(
    cash_flows: list[float],
    discount_rate: float,
    terminal_growth_rate: float = 0.0,
    shares_outstanding: int = 0,
) -> dict:
    """Compute DCF valuation.

    Args:
        cash_flows: Projected free cash flows for each year.
        discount_rate: WACC or required rate of return (e.g. 0.10 for 10%).
        terminal_growth_rate: Perpetual growth rate for terminal value (e.g. 0.02).
        shares_outstanding: If provided, computes per-share intrinsic value.

    Returns:
        Dict with full breakdown of the valuation.
    """
    if discount_rate <= 0:
        return {"error": "Discount rate must be positive"}
    if terminal_growth_rate >= discount_rate:
        return {"error": "Terminal growth rate must be less than discount rate"}

    n = len(cash_flows)
    pv_cash_flows = []
    for i, cf in enumerate(cash_flows):
        year = i + 1
        pv = cf / ((1 + discount_rate) ** year)
        pv_cash_flows.append({
            "year": year,
            "cash_flow": round(cf, 2),
            "discount_factor": round(1 / ((1 + discount_rate) ** year), 6),
            "present_value": round(pv, 2),
        })

    sum_pv = sum(item["present_value"] for item in pv_cash_flows)

    # Terminal value using Gordon Growth Model
    terminal_value = 0.0
    pv_terminal = 0.0
    if terminal_growth_rate > 0 and n > 0:
        last_cf = cash_flows[-1]
        terminal_value = (last_cf * (1 + terminal_growth_rate)) / (discount_rate - terminal_growth_rate)
        pv_terminal = terminal_value / ((1 + discount_rate) ** n)

    enterprise_value = sum_pv + pv_terminal

    result = {
        "inputs": {
            "cash_flows": [round(cf, 2) for cf in cash_flows],
            "discount_rate": discount_rate,
            "terminal_growth_rate": terminal_growth_rate,
            "projection_years": n,
        },
        "yearly_breakdown": pv_cash_flows,
        "sum_pv_cash_flows": round(sum_pv, 2),
        "terminal_value": round(terminal_value, 2),
        "pv_terminal_value": round(pv_terminal, 2),
        "enterprise_value": round(enterprise_value, 2),
    }

    if shares_outstanding > 0:
        result["shares_outstanding"] = shares_outstanding
        result["intrinsic_value_per_share"] = round(enterprise_value / shares_outstanding, 2)

    # Sensitivity table: ±2% on discount rate
    sensitivities = []
    for delta in [-0.02, -0.01, 0, 0.01, 0.02]:
        dr = discount_rate + delta
        if dr <= 0 or dr <= terminal_growth_rate:
            continue
        pv_sum = sum(cf / ((1 + dr) ** (i + 1)) for i, cf in enumerate(cash_flows))
        tv = 0
        if terminal_growth_rate > 0 and n > 0:
            tv = (cash_flows[-1] * (1 + terminal_growth_rate)) / (dr - terminal_growth_rate)
            tv = tv / ((1 + dr) ** n)
        ev = pv_sum + tv
        entry = {"discount_rate": round(dr, 4), "enterprise_value": round(ev, 2)}
        if shares_outstanding > 0:
            entry["per_share"] = round(ev / shares_outstanding, 2)
        sensitivities.append(entry)

    result["sensitivity"] = sensitivities
    return result


def project_cash_flows(base: float, growth_rate: float, years: int) -> list[float]:
    """Project cash flows from a base amount using a constant growth rate."""
    flows = []
    cf = base
    for _ in range(years):
        flows.append(round(cf, 2))
        cf *= (1 + growth_rate)
    return flows


def main():
    parser = argparse.ArgumentParser(description="DCF Valuation Calculator")
    parser.add_argument("--cash-flows", nargs="+", type=float, help="Projected free cash flows by year")
    parser.add_argument("--discount-rate", type=float, help="WACC / discount rate (e.g. 0.10)")
    parser.add_argument("--terminal-growth", type=float, default=0.0, help="Terminal growth rate (e.g. 0.02)")
    parser.add_argument("--shares", type=int, default=0, help="Shares outstanding for per-share value")
    parser.add_argument("--growth-rate", type=float, help="Growth rate to project cash flows from last value")
    parser.add_argument("--years", type=int, default=5, help="Total projection years (with --growth-rate)")
    parser.add_argument("--from-json", metavar="FILE", help="Read inputs from JSON file")
    args = parser.parse_args()

    if args.from_json:
        with open(args.from_json) as f:
            data = json.load(f)
        result = calculate_dcf(
            cash_flows=data["cash_flows"],
            discount_rate=data["discount_rate"],
            terminal_growth_rate=data.get("terminal_growth_rate", 0.0),
            shares_outstanding=data.get("shares_outstanding", 0),
        )
    else:
        if not args.cash_flows or args.discount_rate is None:
            parser.error("--cash-flows and --discount-rate are required (or use --from-json)")

        cash_flows = list(args.cash_flows)
        if args.growth_rate is not None:
            # Extend from last provided cash flow
            base = cash_flows[-1] * (1 + args.growth_rate)
            extra_years = args.years - len(cash_flows)
            if extra_years > 0:
                cash_flows.extend(project_cash_flows(base, args.growth_rate, extra_years))

        result = calculate_dcf(
            cash_flows=cash_flows,
            discount_rate=args.discount_rate,
            terminal_growth_rate=args.terminal_growth,
            shares_outstanding=args.shares,
        )

    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
