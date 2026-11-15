#!/usr/bin/env python3
"""Charts for the DSH confined vs unconfined pilot.

Scope: the hosted DSH runs only. The local 4B (qwen3.5:4b) pilot is excluded
because the model is too weak for the classification task: it produced 3/10
correct root causes, which makes arm-to-arm comparison meaningless there.

Data: reports/agent-benchmark/dsh-pilot-20261115/records.json
      deploy/targets/target-app/scenarios/*.yaml  (ground truth, for context)

Accuracy is saturated (10/10 root cause, 10/10 failure mode), so the charts
concentrate on the things that still differ: token cost, wall time, tool-call
count, scope expansions, and route diagnosis.

No third-party dependencies: emits hand-built SVG.
"""

import json
import os
import re
import glob
from collections import OrderedDict

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
OUT = os.path.dirname(os.path.abspath(__file__))

# ---------------------------------------------------------------- palette
INK = "#11161d"
MUTED = "#6b7785"
GRID = "#e6ebf0"
BG = "#ffffff"
CONF = "#2f6fb3"        # confined arm
UNCONF = "#d08a3a"      # unconfined arm
CONF_FILL = "#dce8f4"
UNCONF_FILL = "#f7e6cf"
GOOD = "#2f855a"
BAD = "#c0392b"
NEUTRAL = "#c8d0d8"

FONT = "'Inter','Helvetica Neue',Helvetica,Arial,sans-serif"

SCEN = OrderedDict([
    ("catalog-http-500", "catalog-http-500"),
    ("auth-login-latency", "auth-login-latency"),
    ("external-payment-outage", "external-payment"),
    ("postgres-outage", "postgres-outage"),
    ("catalog-crashloop", "catalog-crashloop"),
])

# Scenarios whose root cause starts OUTSIDE the initial suspect region and so
# require an audited expansion to reach.
DEPENDENCY = {"external-payment-outage", "postgres-outage", "catalog-crashloop"}


def esc(s):
    return str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def svg_open(w, h, title, sub=""):
    p = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" '
         f'viewBox="0 0 {w} {h}" font-family="{FONT}">',
         f'<rect width="{w}" height="{h}" fill="{BG}"/>',
         f'<text x="30" y="40" font-size="19" font-weight="600" fill="{INK}">{esc(title)}</text>']
    if sub:
        p.append(f'<text x="30" y="62" font-size="12.5" fill="{MUTED}">{esc(sub)}</text>')
    return p


def text(x, y, s, size=11, fill=INK, anchor="start", weight="400"):
    return (f'<text x="{x:.1f}" y="{y:.1f}" font-size="{size}" fill="{fill}" '
            f'text-anchor="{anchor}" font-weight="{weight}">{esc(s)}</text>')


def rect(x, y, w, h, fill, rx=3, opacity=1.0):
    return (f'<rect x="{x:.1f}" y="{y:.1f}" width="{max(w,0):.1f}" height="{max(h,0):.1f}" '
            f'fill="{fill}" rx="{rx}" opacity="{opacity}"/>')


def line(x1, y1, x2, y2, stroke=GRID, width=1, dash=None):
    d = f' stroke-dasharray="{dash}"' if dash else ""
    return (f'<line x1="{x1:.1f}" y1="{y1:.1f}" x2="{x2:.1f}" y2="{y2:.1f}" '
            f'stroke="{stroke}" stroke-width="{width}"{d}/>')


def legend(x, y, items, size=11):
    out, cx = [], x
    for label, color in items:
        out.append(rect(cx, y - 9, 11, 11, color, rx=2))
        out.append(text(cx + 17, y, label, size, INK))
        cx += 17 + 6.4 * len(label) + 24
    return out


def save(name, parts):
    parts.append("</svg>")
    with open(os.path.join(OUT, name), "w") as f:
        f.write("\n".join(parts))
    print("wrote", name)


def short(sid):
    return (sid.replace("catalog-", "").replace("-outage", "")
            .replace("-latency", "").replace("external-payment", "external"))


# ---------------------------------------------------------------- data
def load():
    p = os.path.join(ROOT, "reports/agent-benchmark/dsh-pilot-20261115/records.json")
    raw = json.load(open(p))
    rows = []
    for r in raw["records"]:
        rows.append({
            "scenario": r["scenario"],
            "arm": r["arm"],
            "correct": r["correct"],
            "route": r["route_match"],
            "mode": r["mode_match"],
            "duration": r["duration_s"],
            "calls": r["tool_calls"],
            "total": r["total_tokens"],
            "input": r["input_tokens"],
            "output": r["output_tokens"],
            "cache": r["cache_read_tokens"],
            "fresh": r["input_tokens"] + r["output_tokens"],
            "violations": r.get("violations", 0),
            "expanded": bool(r.get("expanded")),
        })
    return rows, raw.get("meta", {})


def by(rows, sid, arm):
    for r in rows:
        if r["scenario"] == sid and r["arm"] == arm:
            return r
    return None


def arm_total(rows, arm, key):
    return sum(r[key] for r in rows if r["arm"] == arm)


# ================================================================ charts
def chart_headline(rows):
    """Totals per arm: a compact scorecard of where the two arms differ."""
    w, h = 1020, 640
    parts = svg_open(w, h, "Confined vs unconfined: headline totals",
                     "Five scenarios per arm, one repetition each. Accuracy is saturated, so cost separates the arms.")
    gx, gy = 220, 140
    row_h = 54
    metrics = [
        ("Root cause correct", "correct", "count"),
        ("Failure mode correct", "mode", "count"),
        ("Route correct", "route", "count"),
        ("Scope violations", "violations", "count"),
        ("Tool calls", "calls", "sum"),
        ("Wall time (s)", "duration", "avg"),
        ("Total tokens", "total", "sum"),
        ("Fresh tokens (in+out)", "fresh", "sum"),
    ]
    bw = 470
    for idx, (label, key, agg) in enumerate(metrics):
        y = gy + idx * row_h
        parts.append(text(gx - 18, y + 22, label, 11.5, INK, "end", "500"))
        vals = {}
        for arm in ("confined", "unconfined"):
            if agg == "count":
                vals[arm] = (sum(1 for r in rows if r["arm"] == arm and r[key]), 5)
            elif agg == "sum":
                vals[arm] = (arm_total(rows, arm, key), None)
            else:
                vals[arm] = (arm_total(rows, arm, key) / max(sum(1 for r in rows if r["arm"] == arm), 1), None)
        vmax = max(v[0] for v in vals.values()) or 1
        for j, (arm, color) in enumerate([("confined", CONF), ("unconfined", UNCONF)]):
            v, denom = vals[arm]
            frac = (v / denom) if denom else (v / vmax)
            yy = y + (4 if j == 0 else 26)
            parts.append(rect(gx, yy, bw * frac, 16, color, rx=2, opacity=0.85))
            lab = f"{v}{'/'+str(denom) if denom else ''}"
            if key == "duration":
                lab = f"{v:.0f}s total"
            if key in ("total", "calls", "fresh"):
                lab = f"{v:,.0f}"
            parts.append(text(gx + bw * frac + 10, yy + 13, lab, 10.5, color, "start", "600"))
        # separator
        parts.append(line(gx - 160, y + 50, gx + bw + 130, y + 50, "#f0f3f6"))
    parts += legend(gx, gy - 26, [("confined", CONF), ("unconfined", UNCONF)])
    parts.append(text(30, h - 56,
                      "Confined wins on wall time and fresh tokens; unconfined edges ahead on total tool calls.",
                      10.5, MUTED))
    parts.append(text(30, h - 38,
                      "Both arms are 5/5 accurate, so these differences are pure cost.",
                      10.5, MUTED))
    save("01-headline.svg", parts)


def chart_per_scenario_tokens(rows):
    """Token cost by scenario, grouped."""
    w, h = 1020, 500
    parts = svg_open(w, h, "Token cost per scenario",
                     "Total tokens including prompt-cache reads. Lower is better.")
    gx, gy, gw, gh = 210, 140, 640, 260
    vmax = 900000
    for i in range(0, 5):
        v = vmax * i / 4.0
        y = gy + gh - gh * (i / 4.0)
        parts.append(line(gx, y, gx + gw, y, GRID))
        parts.append(text(gx + gw + 12, y + 4, f"{v/1000:.0f}k", 10, MUTED, "start"))
    parts += legend(gx, gy - 22, [("confined", CONF), ("unconfined", UNCONF)])
    slot = gw / len(SCEN)
    bw = slot * 0.22
    for idx, sid in enumerate(SCEN):
        base = gx + slot * (idx + 0.5)
        for j, (arm, color) in enumerate([("confined", CONF), ("unconfined", UNCONF)]):
            r = by(rows, sid, arm)
            v = r["total"]
            bh = gh * (v / vmax)
            x = base - bw - 3 + j * (bw + 6)
            parts.append(rect(x, gy + gh - bh, bw, bh, color, rx=2))
            parts.append(text(x + bw / 2, gy + gh - bh - 7, f"{v/1000:.0f}k", 9.5, INK, "middle", "600"))
        parts.append(text(base, gy + gh + 22, short(sid), 10.5, INK, "middle", "500"))
    parts.append(line(gx, gy + gh, gx + gw, gy + gh, MUTED, 1))
    parts.append(text(gx + gw / 2, gy + gh + 52, "Confined used fewer tokens in 4 of 5 scenarios",
                      10.5, MUTED, "middle"))
    save("02-tokens-per-scenario.svg", parts)


def chart_token_split(rows):
    """Stacked bars: cache read vs fresh work."""
    w, h = 1020, 500
    parts = svg_open(w, h, "Where the tokens went",
                     "Prompt-cache reads dominate the total. Fresh work (input + output) is the model's actual labour.")
    gx, gy, gw, gh = 210, 140, 640, 260
    vmax = 900000
    for i in range(0, 5):
        v = vmax * i / 4.0
        y = gy + gh - gh * (i / 4.0)
        parts.append(line(gx, y, gx + gw, y, GRID))
        parts.append(text(gx + gw + 12, y + 4, f"{v/1000:.0f}k", 10, MUTED, "start"))
    parts += legend(gx, gy - 22, [("cache read", "#b9c4cf"), ("input", "#4a7fb0"), ("output", "#e8a33d")])
    slot = gw / len(SCEN)
    bw = slot * 0.26
    for idx, sid in enumerate(SCEN):
        base = gx + slot * (idx + 0.5)
        for j, arm in enumerate(["confined", "unconfined"]):
            r = by(rows, sid, arm)
            x = base - bw - 3 + j * (bw + 6)
            y0 = gy + gh
            for k, (val, color) in enumerate([(r["cache"], "#b9c4cf"),
                                              (r["input"], "#4a7fb0"),
                                              (r["output"], "#e8a33d")]):
                bh = gh * (val / vmax)
                y0 -= bh
                parts.append(rect(x, y0, bw, bh, color, rx=1))
        parts.append(text(base, gy + gh + 22, short(sid), 10.5, INK, "middle", "500"))
    parts.append(line(gx, gy + gh, gx + gw, gy + gh, MUTED, 1))
    parts.append(text(gx + gw / 2, gy + gh + 52, "Left bar = confined, right bar = unconfined",
                      10.5, MUTED, "middle"))
    save("03-token-split.svg", parts)


def chart_duration(rows):
    """Wall time per scenario."""
    w, h = 1020, 500
    parts = svg_open(w, h, "Wall-clock time per run",
                     "Seconds from run start to final diagnosis. Lower is better.")
    gx, gy, gw, gh = 210, 140, 640, 260
    vmax = 200
    for i in range(0, 5):
        v = vmax * i / 4.0
        y = gy + gh - gh * (i / 4.0)
        parts.append(line(gx, y, gx + gw, y, GRID))
        parts.append(text(gx + gw + 12, y + 4, f"{v:.0f}s", 10, MUTED, "start"))
    parts += legend(gx, gy - 22, [("confined", CONF), ("unconfined", UNCONF)])
    slot = gw / len(SCEN)
    bw = slot * 0.22
    faster = 0
    for idx, sid in enumerate(SCEN):
        base = gx + slot * (idx + 0.5)
        c, u = by(rows, sid, "confined"), by(rows, sid, "unconfined")
        if c["duration"] < u["duration"]:
            faster += 1
        for j, (r, color) in enumerate([(c, CONF), (u, UNCONF)]):
            bh = gh * (r["duration"] / vmax)
            x = base - bw - 3 + j * (bw + 6)
            parts.append(rect(x, gy + gh - bh, bw, bh, color, rx=2))
            parts.append(text(x + bw / 2, gy + gh - bh - 7, f"{r['duration']:.0f}", 9.5, INK, "middle", "600"))
        parts.append(text(base, gy + gh + 22, short(sid), 10.5, INK, "middle", "500"))
    parts.append(line(gx, gy + gh, gx + gw, gy + gh, MUTED, 1))
    parts.append(text(gx + gw / 2, gy + gh + 52,
                      f"Confined was faster in {faster} of 5 scenarios", 10.5, MUTED, "middle"))
    save("04-duration.svg", parts)


def chart_calls(rows):
    """Tool calls, paired dumbbell to show direction."""
    w, h = 1020, 470
    parts = svg_open(w, h, "Tool calls per run",
                     "Dots show each arm; the connector shows the difference. Fewer calls with equal accuracy is better.")
    gx, gy, gw, gh = 210, 150, 560, 220
    vmax = 32
    for i in range(0, 5):
        v = vmax * i / 4.0
        y = gy + gh - gh * (i / 4.0)
        parts.append(line(gx, y, gx + gw, y, GRID))
        parts.append(text(gx + gw + 12, y + 4, f"{v:.0f}", 10, MUTED, "start"))
    parts += legend(gx, gy - 22, [("confined", CONF), ("unconfined", UNCONF)])
    row_h = gh / len(SCEN)
    for idx, sid in enumerate(SCEN):
        y = gy + idx * row_h + row_h / 2
        parts.append(text(gx - 16, y + 4, short(sid), 10.5, INK, "end", "500"))
        c, u = by(rows, sid, "confined"), by(rows, sid, "unconfined")
        xc = gx + gw * (c["calls"] / vmax)
        xu = gx + gw * (u["calls"] / vmax)
        parts.append(line(xc, y, xu, y, "#b0bac4", 2.5))
        parts.append(f'<circle cx="{xc:.1f}" cy="{y:.1f}" r="6" fill="{CONF}"/>')
        parts.append(f'<circle cx="{xu:.1f}" cy="{y:.1f}" r="6" fill="{UNCONF}"/>')
        lo, hi = (xc, xu) if xc < xu else (xu, xc)
        parts.append(text(lo - 10, y + 4, f"{min(c['calls'], u['calls'])}", 9.5, MUTED, "end"))
        parts.append(text(hi + 10, y + 4, f"{max(c['calls'], u['calls'])}", 9.5, MUTED, "start"))
    parts.append(line(gx, gy + gh, gx + gw, gy + gh, MUTED, 1))
    parts.append(text(30, h - 24,
                      "Confined used fewer calls in 3 of 5. On postgres it used nearly double (30 vs 16) for the same answer.",
                      10.5, MUTED))
    save("05-tool-calls.svg", parts)


def chart_quality(rows):
    """Diagnosis matrix, DSH only."""
    w, h = 940, 470
    parts = svg_open(w, h, "Diagnosis quality by arm",
                     "Each cell is a pass/fail against scenario ground truth. Route is the only column with misses.")
    x0, y0 = 250, 150
    cw, ch = 110, 46
    cols = ["root cause", "failure mode", "route"]
    arms = [("confined", CONF), ("unconfined", UNCONF)]
    cx = x0
    for aname, color in arms:
        for ci, c in enumerate(cols):
            if ci == 0:
                parts.append(text(cx + cw * 1.5, y0 - 26, aname, 11, color, "middle", "700"))
            parts.append(text(cx + cw / 2, y0 - 10, c, 9.5, MUTED, "middle"))
            cx += cw
    for idx, sid in enumerate(SCEN):
        y = y0 + idx * ch
        parts.append(text(x0 - 18, y + ch / 2 + 4, short(sid), 11, INK, "end", "500"))
        if sid in DEPENDENCY:
            parts.append(text(x0 - 18, y + ch / 2 + 17, "needs expansion", 8.5, UNCONF, "end"))
        cx = x0
        for aname, _ in arms:
            r = by(rows, sid, aname)
            for c in cols:
                key = {"root cause": "correct", "failure mode": "mode", "route": "route"}[c]
                ok = r[key]
                parts.append(rect(cx + 3, y + 3, cw - 6, ch - 6, GOOD if ok else BAD, rx=3,
                                  opacity=0.18 if ok else 0.20))
                parts.append(text(cx + cw / 2, y + ch / 2 + 5,
                                  "\u2713" if ok else "\u2717", 14, GOOD if ok else BAD, "middle", "700"))
                cx += cw
        parts.append(line(x0, y, cx, y, GRID))
    parts.append(line(x0, y0 + len(SCEN) * ch, cx, y0 + len(SCEN) * ch, MUTED, 1))
    parts.append(text(30, h - 42,
                      "The two route misses are the same scenario in both arms, and are a ground-truth ambiguity:",
                      10.5, MUTED))
    parts.append(text(30, h - 26,
                      "the injected slow route is POST /api/auth/login, but the observed 5xx lands on POST /api/payments.",
                      10.5, MUTED))
    save("06-quality.svg", parts)


def chart_expansion(rows):
    """Expansion on dependency scenarios."""
    w, h = 1020, 470
    parts = svg_open(w, h, "Expansion on dependency scenarios",
                     "Root cause starts outside the initial region; reaching it requires an audited one-hop expansion.")
    gx, gy, gw, gh = 210, 140, 520, 220
    parts += legend(gx, gy - 22, [("expanded", GOOD), ("did not expand", NEUTRAL)])
    bw = gw / len(DEPENDENCY) * 0.30
    for idx, sid in enumerate([s for s in SCEN if s in DEPENDENCY]):
        base = gx + gw * (idx + 0.5) / 3
        for j, (arm, color) in enumerate([("confined", CONF), ("unconfined", UNCONF)]):
            r = by(rows, sid, arm)
            x = base - bw - 4 + j * (bw + 8)
            if r["expanded"]:
                parts.append(rect(x, gy, bw, gh, GOOD, rx=2))
                parts.append(text(x + bw / 2, gy - 8, "expanded", 10, GOOD, "middle", "600"))
            else:
                # zero-height marker: a flat rule on the baseline
                parts.append(rect(x, gy + gh - 4, bw, 4, NEUTRAL, rx=2))
                parts.append(text(x + bw / 2, gy + gh - 12, "none", 10, MUTED, "middle", "600"))
            parts.append(text(x + bw / 2, gy + gh + 20, arm, 9.5, color, "middle", "600"))
        parts.append(text(base, gy + gh + 42, short(sid), 10.5, INK, "middle", "500"))
    parts.append(line(gx, gy + gh, gx + gw, gy + gh, MUTED, 1))
    parts.append(text(gx + gw + 30, gy + 30, "Confined expanded on 2/3.", 11, INK, "start", "600"))
    parts.append(text(gx + gw + 30, gy + 52, "Unconfined expanded on 0/3,", 11, INK, "start"))
    parts.append(text(gx + gw + 30, gy + 70, "yet still answered correctly —", 11, INK, "start"))
    parts.append(text(gx + gw + 30, gy + 88, "it had the whole graph already.", 11, INK, "start"))
    save("07-expansion.svg", parts)


def chart_efficiency(rows):
    """Cost per correct diagnosis, the summary trade."""
    w, h = 1020, 560
    parts = svg_open(w, h, "Cost per correct diagnosis",
                     "Totals across the five scenarios. Both arms scored 5/5 on root cause, so lower cost is strictly better.")
    gx, gy = 250, 150
    bw = 480
    arms = [("confined", CONF), ("unconfined", UNCONF)]
    metrics = [("Total tokens", lambda a: arm_total(rows, a, "total"), 3_600_000, "k"),
               ("Fresh tokens", lambda a: arm_total(rows, a, "fresh"), 500_000, "k"),
               ("Tool calls", lambda a: arm_total(rows, a, "calls"), 130, ""),
               ("Wall time (s)", lambda a: arm_total(rows, a, "duration"), 570, "s")]
    row_h = 74
    for idx, (label, fn, vmax, unit) in enumerate(metrics):
        y = gy + idx * row_h
        parts.append(text(gx - 18, y + 22, label, 11.5, INK, "end", "500"))
        for j, (arm, color) in enumerate(arms):
            v = fn(arm)
            frac = v / vmax
            yy = y + (2 if j == 0 else 26)
            parts.append(rect(gx, yy, bw * frac, 18, color, rx=2, opacity=0.9))
            shown = f"{v/1000:,.0f}k" if unit == "k" else f"{v:,.0f}{unit}"
            parts.append(text(gx + bw * frac + 10, yy + 14, shown, 11, color, "start", "600"))
            parts.append(text(gx - 4, yy + 14, "", 9, MUTED, "end"))
    # delta summary
    dt = arm_total(rows, "unconfined", "total") - arm_total(rows, "confined", "total")
    df = arm_total(rows, "unconfined", "fresh") - arm_total(rows, "confined", "fresh")
    parts.append(text(gx, gy + 4 * row_h + 24,
                      f"Unconfined cost {dt/1000:+.0f}k more total tokens and {df/1000:+.0f}k more fresh tokens,",
                      11, INK))
    parts.append(text(gx, gy + 4 * row_h + 42,
                      "for the same 5/5 root-cause and failure-mode accuracy.", 11, INK))
    parts += legend(gx, gy - 24, [("confined", CONF), ("unconfined", UNCONF)])
    save("08-efficiency.svg", parts)


def chart_deltas(rows):
    """Signed per-scenario deltas (unconfined minus confined)."""
    w, h = 1020, 660
    parts = svg_open(w, h, "Deltas per scenario (unconfined − confined)",
                     "Bars right of centre mean unconfined used more. Negative is better for cost metrics.")
    specs = [
        ("Total tokens", lambda r: r["total"] / 1000.0, "k tokens", 420),
        ("Wall time", lambda r: r["duration"], "seconds", 120),
        ("Tool calls", lambda r: r["calls"], "calls", 16),
        ("Fresh tokens", lambda r: r["fresh"] / 1000.0, "k tokens", 70),
    ]
    panel_h = (h - 150) / len(specs)
    gx, gw = 300, 440
    for pi, (label, fn, unit, vmax) in enumerate(specs):
        top = 118 + pi * panel_h
        parts.append(text(gx - 150, top + 14, label, 11.5, INK, "start", "600"))
        # axis
        zero = gx + gw / 2
        parts.append(line(zero, top, zero, top + panel_h - 26, MUTED, 1))
        row_h = (panel_h - 34) / len(SCEN)
        for idx, sid in enumerate(SCEN):
            y = top + 8 + idx * row_h
            c, u = by(rows, sid, "confined"), by(rows, sid, "unconfined")
            d = fn(u) - fn(c)
            scaled = (d / vmax) * (gw / 2)
            col = BAD if d > 0 else GOOD
            if abs(scaled) < 2:
                parts.append(line(zero - 2, y, zero + 2, y, MUTED, 2))
            else:
                x = zero if d > 0 else zero + scaled
                parts.append(rect(x, y - 6, abs(scaled), 12, col, rx=2, opacity=0.85))
            tx = zero + scaled + (8 if d >= 0 else -8)
            anchor = "start" if d >= 0 else "end"
            parts.append(text(tx, y + 4, f"{d:+.0f}", 9.5, col, anchor, "600"))
            parts.append(text(gx - 20, y + 4, short(sid), 9.5, MUTED, "end"))
        parts.append(line(gx, top + panel_h - 26, gx + gw, top + panel_h - 26, GRID))
    parts.append(text(gx + gw / 2, h - 14, "unconfined − confined", 10.5, MUTED, "middle"))
    save("09-deltas.svg", parts)


def write_readme(rows):
    def tot(arm, k):
        return sum(r[k] for r in rows if r["arm"] == arm)
    lines = [
        "# DSH confined vs unconfined — charts",
        "",
        "Charts for the hosted DSH pilot only. The local 4B (`qwen3.5:4b`) runs are",
        "**excluded**: that model scored 3/10 on root cause, which is too weak for",
        "arm-to-arm comparison to mean anything.",
        "",
        f"Dataset: {len(rows)} runs ({len(SCEN)} scenarios × 2 arms), one repetition.",
        "Source: `reports/agent-benchmark/dsh-pilot-20261115/records.json`.",
        "",
        "| Chart | Question it answers |",
        "| --- | --- |",
        "| `01-headline.svg` | How do the arms compare on a single scorecard? |",
        "| `02-tokens-per-scenario.svg` | Where did the token advantage come from? |",
        "| `03-token-split.svg` | How much was cache read vs actual work? |",
        "| `04-duration.svg` | Which arm was faster, and where? |",
        "| `05-tool-calls.svg` | Did confinement reduce tool calls? |",
        "| `06-quality.svg` | Which diagnoses were right or wrong? |",
        "| `07-expansion.svg` | Did confined expand to reach dependency faults? |",
        "| `08-efficiency.svg` | What is the cost per correct diagnosis? |",
        "| `09-deltas.svg` | Per-scenario signed difference, all metrics |",
        "",
        "## Headline",
        "",
        "| Metric | confined | unconfined |",
        "| --- | ---: | ---: |",
        f"| Root cause correct | 5/5 | 5/5 |",
        f"| Failure mode correct | 5/5 | 5/5 |",
        f"| Route correct | 4/5 | 4/5 |",
        f"| Expansions | 2 | 0 |",
        f"| Tool calls | {tot('confined','calls')} | {tot('unconfined','calls')} |",
        f"| Total tokens | {tot('confined','total'):,} | {tot('unconfined','total'):,} |",
        f"| Fresh tokens | {tot('confined','fresh'):,} | {tot('unconfined','fresh'):,} |",
        f"| Wall time | {tot('confined','duration'):.0f}s | {tot('unconfined','duration'):.0f}s |",
        "",
        "## Caveats",
        "",
        "- **One repetition per scenario per arm.** No medians, no confidence",
        "  intervals, no significance. Treat differences as directions, not effects.",
        "- **Accuracy is saturated.** 5/5 root cause and 5/5 failure mode in both",
        "  arms means accuracy cannot rank the arms here. Only cost does.",
        "- **Token totals include prompt-cache reads.** They reflect session usage,",
        "  not model work. `Fresh tokens` (input + output) is the fairer measure.",
        "- **Route misses are a ground-truth ambiguity, not agent error.** The",
        "  injected slow route is `POST /api/auth/login`; the observed 5xx lands on",
        "  `POST /api/payments` because the gateway calls auth first. Both arms miss",
        "  it identically.",
        "- **Expansion is confounded with scenario design.** Confined *must* expand",
        "  on dependency scenarios; unconfined has the whole graph already. `0/3`",
        "  for unconfined means expansion was unnecessary, not that it was refused.",
        "",
        "## Reproduce",
        "",
        "```bash",
        "python3 reports/agent-benchmark/charts/generate.py",
        "```",
        "",
        "No third-party dependencies; emits SVG directly.",
        "",
    ]
    with open(os.path.join(OUT, "README.md"), "w") as f:
        f.write("\n".join(lines))
    print("wrote README.md")


def main():
    rows, meta = load()
    assert len(rows) == 10, f"expected 10 DSH runs, got {len(rows)}"
    chart_headline(rows)
    chart_per_scenario_tokens(rows)
    chart_token_split(rows)
    chart_duration(rows)
    chart_calls(rows)
    chart_quality(rows)
    chart_expansion(rows)
    chart_efficiency(rows)
    chart_deltas(rows)
    write_readme(rows)


if __name__ == "__main__":
    main()
