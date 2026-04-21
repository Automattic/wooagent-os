Stop running the store. Start running the business.

Designers: @elizaan36 + @nevenailic1013 | Project: WooAgent OS

---

## The Problem

Running a WooCommerce store is a second job inside your actual job.

A solo operator or small team is expected to write product copy, watch competitor pricing,

track stock levels, reconcile payouts, answer customer questions, and generate reports for

their accountant, all on top of actually running the business. Most of this work is

repetitive and tedious, and nearly all of it is the kind of thing an AI is

good at.

But "use AI to help with your store" today means opening a chat window, writing a long

prompt from scratch, getting an answer, copying it somewhere, and doing it again tomorrow.

There's no memory or coordination, and no trail.

The gap isn't the AI itself, but the system around the AI.

---

## What We're Building

WooAgent OS is an open-source, local-first agent operating system for WooCommerce store

operators. It runs a fleet of specialized AI agents—Marketing, Pricing, Inventory,

Accounting, Reporting, and Sales Support—that continuously work against your live store. A

meta-agent called the Chief of Staff coordinates them.

The key distinction from anything else in this space: agents propose, operators approve.

Nothing gets written to the store without the operator seeing a diff and saying yes.

The interface is a kanban board: a command center where every unit of agent work is a

trackable issue. Operators see exactly what agents are queuing, running, waiting on, or

completing. They can jump in at any step.

The system is local-first (your data stays on your machine), model-agnostic (connect it to

Claude, Gemini, GPT, or a local model you run yourself), and MCP-native (it connects to your

store through WordPress 7.0's new Abilities API, so it automatically works with

WooCommerce, Yoast, ACF, Gravity Forms, and 70+ other plugins—no custom integrations

required).

---

## The Design Surface

Two of us are pairing on the user-facing experience, so everything the operator touches. This

is the part that will make or break the project, because the technical capability only

matters if people trust it, understand it, and actually use it.

The core design challenge is this: how do you make AI work feel legible, controllable, and

trustworthy to a store owner who didn't sign up to manage a robot?

We're focused on four areas:

1. **Onboarding and first run**The product goal is: connect a store, see the first agent-produced issue in under 10 minuteson a fresh machine. The first-run experience has to support connecting to the store via MCP, discovering plugins and abilities, configuring a model provider, and deploying the default agent fleet without being overwhelming for a first-time user.
2. **The kanban board**This is the primary screen. Every agent action is an issue. The columns map to a naturalworkflow: Backlog → Todo → In Progress → In Review → Done. The design work here is aboutinformation hierarchy at a glance: what's the agent doing right now, what needs myattention, what just happened? We're designing for the operator who checks in for 15 minutesin the morning, not someone monitoring a dashboard all day.
3. **The review and approval moment**This is the most critical interaction in the product. When an agent proposes a change—aprice update, a product description rewrite, a draft email—it lands in In Review. Theoperator sees a before/after diff and approves or rejects. This has to be fast, clear, andtrustworthy so people understand what they're approving, rather than either rubber-stampingeverything or rejecting everything.
4. **The ability browser and agent roster**Less frequent, but important for power users. Operators need to understand which agents are active, what each one can do, which store plugins they're connected to, and how to tune or disable them. This is the "under the hood" view that should feel like a control center.

---

## High-Level Design Details

A few things we know shape the direction:

- Familiar mental model, new context. Kanban is well-understood from tools like Linear andJira. We're not inventing a new interaction pattern — we're applying a trusted one to a newproblem. The challenge is that the "cards" here are agent actions, not human tasks, so someconventions need to shift.
- The operator is always the final decision-maker. The entire UX should reinforce this.Agents surface work; humans decide. The visual language should make this hierarchy obviouswithout making the agents feel like a burden to manage.
- Every action is auditable. Every model call, every ability invocation, every change islogged. The run log is a first-class feature. We need to make "show me exactly what theagent did" a satisfying and fast experience, not a wall of JSON.
- The agent fleet has personalities. Each persona has a name, a mandate, a preferred model,and a set of abilities. Giving them some character — not cartoon mascots, but distinctidentities — helps operators develop an intuition for who does what. This is also just morefun to use.
- Local-first means the UI is served locally. No cloud account required. The web app runs onlocalhost:7777. This constrains and simplifies some things (no auth flows to design, noloading states from remote APIs) and opens others (we can be aggressive about real-timeupdates since everything is on the same machine).

---

## How We'll Know It's Working

The PRD gives us a clear set of measurable targets. As designers, these are the ones we own

the most:

- **Time-to-first-issue**. Under 10 minutes from install to first agent-produced issue
- **Approval rate.** Operators approve 70%+ of agent-proposed writes at steady state
- **30-day retention**. Operators running at least one agent daily after a month
- **Ability coverage.** Agents use abilities from 3+ plugins in the first session

---

## Why Now

WordPress 7.0 shipped the Abilities API and MCP Adapter. WooCommerce registers 42 abilities.

Over 70 plugins follow the same pattern. The infrastructure for AI agents to operate on

WordPress stores is already there. What's missing is the opinionated layer on top. The

agent fleet, the work-tracking system, the propose/approve workflow, and the UX that makes all

of it manageable.