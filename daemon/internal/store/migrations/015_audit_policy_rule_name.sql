-- DSGWOO-1282: capture the policy rule name that fired on a denial.
--
-- The PEP's typed denial_reason ("policy_violation") tells the operator
-- *that* a policy denied a call; this column tells them *which* policy.
-- Lookups + dashboards that already filter on denial_reason are
-- unaffected; reads that want the rule name JOIN/SELECT this column
-- separately. NULL for every non-policy-violation row.

ALTER TABLE audit_invocations ADD COLUMN policy_rule_name TEXT;
