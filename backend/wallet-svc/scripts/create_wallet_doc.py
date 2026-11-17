#!/usr/bin/env python3
"""Generate Wallet + Buy Token Word document.

The implementation source of truth is docs/OTT-Wallet-Service-Plan.md
(credit TOKEN_PURCHASE / debit SPEND only; no refunds).
"""

from pathlib import Path

from docx import Document
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.oxml.ns import qn
from docx.shared import Inches, Pt, RGBColor


NAVY = RGBColor(0x1B, 0x36, 0x5D)
ACCENT = RGBColor(0x2E, 0x5A, 0x88)
BODY = RGBColor(0x2D, 0x2D, 0x2D)
WHITE = RGBColor(0xFF, 0xFF, 0xFF)
HEADER_BG = "1B365D"
ALT_ROW = "F3F6FA"


def set_run(run, size=11, bold=False, color=BODY, font="Calibri"):
    run.font.name = font
    run._element.rPr.rFonts.set(qn("w:eastAsia"), font)
    run.font.size = Pt(size)
    run.bold = bold
    run.font.color.rgb = color


def shade_cell(cell, hex_color):
    tc = cell._tc
    tc_pr = tc.get_or_add_tcPr()
    shd = tc_pr.makeelement(
        qn("w:shd"),
        {
            qn("w:fill"): hex_color,
            qn("w:val"): "clear",
        },
    )
    tc_pr.append(shd)


def set_cell_border(cell):
    tc = cell._tc
    tc_pr = tc.get_or_add_tcPr()
    borders = tc_pr.makeelement(qn("w:tcBorders"), {})
    for edge in ("top", "left", "bottom", "right"):
        borders.append(
            tc_pr.makeelement(
                qn(f"w:{edge}"),
                {
                    qn("w:val"): "single",
                    qn("w:sz"): "4",
                    qn("w:space"): "0",
                    qn("w:color"): "C5CDD6",
                },
            )
        )
    tc_pr.append(borders)


def add_heading(doc, text, level=1):
    p = doc.add_paragraph()
    p.paragraph_format.space_before = Pt(16 if level == 1 else 10)
    p.paragraph_format.space_after = Pt(6)
    run = p.add_run(text)
    set_run(run, size=16 if level == 1 else 13, bold=True, color=NAVY)
    return p


def add_body(doc, text):
    p = doc.add_paragraph()
    p.paragraph_format.space_after = Pt(8)
    p.paragraph_format.space_before = Pt(0)
    run = p.add_run(text)
    set_run(run, size=11)
    return p


def add_bullet(doc, text):
    p = doc.add_paragraph(style="List Bullet")
    p.clear()
    p.paragraph_format.space_after = Pt(3)
    run = p.add_run(text)
    set_run(run, size=11)
    return p


FLOW_DIAGRAM = """App                    Orders                   Payment                 Wallet
 |-- create order ------>|                         |                       |
 |<-- order_id+checkout -|                         |                       |
 |-- pay --------------->|------------------------>|                       |
 |                       |<-- webhook PAID --------|                       |
 |                       |-- credit(order_id) ---------------------------->|
 |                       |<-- balance_after -------------------------------|
 |-- poll order -------->| COMPLETED               |                       |
 |<-- new balance -------|                         |                       |"""


def add_code_block(doc, text):
    table = doc.add_table(rows=1, cols=1)
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    cell = table.rows[0].cells[0]
    shade_cell(cell, "F4F6F8")
    set_cell_border(cell)
    cell.text = ""
    first = True
    for line in text.splitlines():
        p = cell.paragraphs[0] if first else cell.add_paragraph()
        first = False
        p.paragraph_format.space_before = Pt(0)
        p.paragraph_format.space_after = Pt(0)
        p.paragraph_format.line_spacing = 1.05
        run = p.add_run(line if line else " ")
        set_run(run, size=8, color=BODY, font="Consolas")
    spacer = doc.add_paragraph()
    spacer.paragraph_format.space_after = Pt(4)
    return table


def fill_table(table, headers, rows):
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    table.autofit = True

    for i, header in enumerate(headers):
        cell = table.rows[0].cells[i]
        cell.text = ""
        p = cell.paragraphs[0]
        run = p.add_run(header)
        set_run(run, size=10, bold=True, color=WHITE)
        shade_cell(cell, HEADER_BG)
        set_cell_border(cell)

    for r_idx, row in enumerate(rows):
        for c_idx, value in enumerate(row):
            cell = table.rows[r_idx + 1].cells[c_idx]
            cell.text = ""
            p = cell.paragraphs[0]
            run = p.add_run(value)
            set_run(run, size=10)
            if r_idx % 2 == 1:
                shade_cell(cell, ALT_ROW)
            set_cell_border(cell)


def add_table(doc, headers, rows, col_widths=None):
    table = doc.add_table(rows=1 + len(rows), cols=len(headers))
    fill_table(table, headers, rows)
    if col_widths:
        for row in table.rows:
            for i, width in enumerate(col_widths):
                row.cells[i].width = Inches(width)
    doc.add_paragraph()
    return table


def build():
    doc = Document()

    section = doc.sections[0]
    section.top_margin = Inches(0.85)
    section.bottom_margin = Inches(0.85)
    section.left_margin = Inches(0.75)
    section.right_margin = Inches(0.75)

    title = doc.add_paragraph()
    title.alignment = WD_ALIGN_PARAGRAPH.LEFT
    title.paragraph_format.space_after = Pt(2)
    run = title.add_run("Wallet Service & Buy Token Flow")
    set_run(run, size=22, bold=True, color=NAVY)

    subtitle = doc.add_paragraph()
    subtitle.paragraph_format.space_after = Pt(4)
    run = subtitle.add_run("In-app coin wallet — purchase tokens only")
    set_run(run, size=12, color=ACCENT)

    meta = doc.add_paragraph()
    meta.paragraph_format.space_after = Pt(12)
    run = meta.add_run("Stack: Go · PostgreSQL · Redis    |    Scope: Wallet + buy tokens (spend / OTT excluded)")
    set_run(run, size=10, color=RGBColor(0x5A, 0x5A, 0x5A))

    # Goal
    add_heading(doc, "1. Goal")
    add_body(
        doc,
        "Build a closed-loop in-app coin wallet. Each user has one wallet. Users buy token packs with real money. Coins are credited only after a verified payment. The same payment must never credit twice. The mobile app never credits the wallet directly.",
    )
    add_bullet(doc, "Create one wallet per user at signup, starting balance 0.")
    add_bullet(doc, "Credit coins only when Orders has a verified payment (webhook or store receipt).")
    add_bullet(doc, "Idempotent credit: retries and duplicate webhooks do not add coins again.")
    add_bullet(doc, "PostgreSQL is the source of truth. Redis is cache and a fast duplicate gate only.")

    # Services
    add_heading(doc, "2. Services")
    add_table(
        doc,
        ["Service", "Responsibility"],
        [
            [
                "React Native app",
                "Create buy order, open checkout, poll order status, show balance. Does not call Wallet credit.",
            ],
            [
                "Orders",
                "Token-pack catalog, order state, payment session, payment webhook, then call Wallet credit.",
            ],
            [
                "Payment gateway / IAP",
                "Collect payment and notify Orders. Does not call Wallet.",
            ],
            [
                "Wallet (this service)",
                "Create wallet, get balance, credit coins, ledger. Go + PostgreSQL + Redis.",
            ],
        ],
        col_widths=[2.0, 4.8],
    )
    # Buy token flow
    add_heading(doc, "3. Buy token flow")
    add_body(
        doc,
        "The app never credits the wallet. Orders creates the order, payment notifies Orders, and Orders credits Wallet. The app then polls until the order is COMPLETED and shows the new balance.",
    )
    add_code_block(doc, FLOW_DIAGRAM)
    add_table(
        doc,
        ["Step", "From → To", "What happens"],
        [
            ["1", "App → Orders", "Create buy-token order."],
            ["2", "Orders → App", "Return order_id and checkout payload."],
            ["3", "App → Payment", "User pays via gateway / IAP."],
            ["4", "Payment → Orders", "Webhook PAID (only trusted credit trigger)."],
            ["5", "Orders → Wallet", "credit(order_id) after verified payment."],
            ["6", "Wallet → Orders", "Return balance_after."],
            ["7", "App → Orders", "Poll order until COMPLETED / CREDITED."],
            ["8", "Orders → App", "Return new balance (or app reads Wallet get-balance)."],
        ],
        col_widths=[0.8, 2.0, 4.0],
    )

    # Tech stack
    add_heading(doc, "4. Tech stack")
    add_table(
        doc,
        ["Layer", "Choice"],
        [
            ["Wallet language", "Go 1.22+"],
            ["HTTP", "chi (net/http)"],
            ["Database", "PostgreSQL (ledger + cached balance)"],
            ["DB driver", "jackc/pgx/v5"],
            ["Migrations", "golang-migrate"],
            ["Cache / idempotency gate", "Redis + redis/go-redis"],
            ["Logging", "log/slog"],
            ["Config", "Environment variables"],
            ["Orders / App", "Existing services + React Native client"],
        ],
        col_widths=[2.4, 4.4],
    )

    # APIs
    add_heading(doc, "5. API endpoints")
    add_heading(doc, "5.1 Wallet (internal — not called by the app for credit)", level=2)
    add_table(
        doc,
        ["Method", "Endpoint", "Purpose"],
        [
            ["POST", "/internal/wallets", "Create wallet for a user (idempotent on user_id)"],
            ["GET", "/internal/wallets/{user_id}", "Get current coin balance"],
            ["POST", "/internal/wallets/{user_id}/credit", "Credit coins after verified payment"],
            ["GET", "/internal/wallets/{user_id}/ledger", "List ledger entries (support / history)"],
        ],
        col_widths=[1.1, 3.2, 2.5],
    )
    add_body(
        doc,
        "Credit requires idempotency_key (example: order:{order_id}:credit), server-side amount, reason TOKEN_PURCHASE, and payment reference_id. Duplicate key returns the original credit, not a second credit.",
    )

    add_heading(doc, "5.2 Orders (buy token only)", level=2)
    add_table(
        doc,
        ["Method", "Endpoint", "Purpose"],
        [
            ["POST", "/orders/token-packs", "Create buy order; returns order_id + checkout payload"],
            ["GET", "/orders/{id}", "Poll order status until CREDITED or FAILED"],
            ["POST", "/orders/webhooks/payment", "Payment / IAP webhook; only trusted trigger to credit"],
        ],
        col_widths=[1.1, 3.2, 2.5],
    )
    add_body(
        doc,
        "App calls Orders only. After payment, Orders calls POST /internal/wallets/{user_id}/credit. The app never calls Wallet credit.",
    )

    # Timeline
    add_heading(doc, "6. Expected timeline")
    add_body(
        doc,
        "Estimate for one backend engineer plus React Native wiring. Spend / OTT is not included.",
    )
    add_table(
        doc,
        ["Phase", "Duration", "Deliverable"],
        [
            [
                "Week 1 — Wallet core",
                "5 days",
                "Go service, Postgres schema, create wallet, get balance, credit + ledger, unique idempotency.",
            ],
            [
                "Week 2 — Buy order flow",
                "5 days",
                "Orders token-pack APIs, mock payment, webhook → Wallet credit, app poll + balance UI.",
            ],
            [
                "Week 3 — Real payment + Redis",
                "5 days",
                "Gateway or IAP webhook, Redis balance cache and idempotency gate, retry on CREDIT_FAILED.",
            ],
            [
                "Week 4 — Harden & handoff",
                "5 days",
                "Auth on internal APIs, tests, duplicate-payment checks, staging sign-off.",
            ],
        ],
        col_widths=[2.2, 1.2, 3.4],
    )
    add_body(
        doc,
        "Total: about 4 weeks to a staging-ready buy-token wallet. Real App Store / Play billing review sits outside this estimate.",
    )

    footer = doc.sections[0].footer.paragraphs[0]
    footer.alignment = WD_ALIGN_PARAGRAPH.LEFT
    run = footer.add_run("Wallet POC  ·  Buy token flow  ·  Confidential")
    set_run(run, size=8, color=RGBColor(0x88, 0x88, 0x88))

    out = Path("/Users/devilghost/Development/mogi/bitbucket/wallet-svc/docs/Wallet-and-Buy-Token-Flow.docx")
    out.parent.mkdir(parents=True, exist_ok=True)
    doc.save(out)
    print(out)


if __name__ == "__main__":
    build()
