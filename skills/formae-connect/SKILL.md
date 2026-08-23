---
name: formae-connect
description: "Use when the user wants to connect a cloud account to their hosted formae installation, or add another one — e.g. 'connect my AWS account', 'give formae access to account 123456789012', 'add a second AWS account', 'formae can't see my resources yet'."
---

# Connect a cloud account

Gives a hosted formae installation permission to manage resources in a cloud
account. For AWS there are two ways to get there now: provisioning directly
from local AWS credentials in one step, or having the user apply a small
CloudFormation stack themselves. Neither path exists yet for Azure or GCP.

## Step 1 — Which cloud?

Ask AWS, Azure, or GCP. Offer all three even though only AWS works today —
that keeps the shape of the product visible rather than looking AWS-only by
accident.

**Azure or GCP:** say plainly that support is not built yet. Do not promise a
date; there is not one to promise.

**AWS:** go to step 2.

## Step 2 — AWS: local credentials, or the console link?

Call `list_aws_profiles`. It is a cheap read and it answers the method question
for you — present what it returns and let the user pick from that, rather than
asking them to choose an approach in the abstract.

Present every profile it returns:

- **A resolved profile** as name and account together, e.g. "work-prod:
  account 111122223333". The account belongs right there: the user is picking
  credentials, not an account, and seeing which account those credentials reach
  is what makes the pick an informed decision about where trust gets
  provisioned. Never ask them to type the account separately, and never show a
  resolved profile without it.
- **An unavailable profile**, with its reason, e.g. "staging: unavailable (SSO
  session expired)". This is not an error to hide — "your SSO session expired"
  is something the user can fix and retry. Show it alongside the resolved ones,
  never drop it.
- **A final option meaning "none of these"**. Always offer it, whether or not
  any profile resolved. It leads to the console-link flow in step 4.

**No local profiles at all is a normal outcome**, not a failure: it just means
the console link is the only path available here. Say that plainly and go
straight to step 4 — there is nothing to choose between.

## Step 3 — A profile was chosen: provision directly

**Say what is about to happen before you call anything.** Unlike the console
link, there is no review step standing between this call and the mutation:
`provision_cloud_role` creates the IAM role immediately, and possibly the
account-global OIDC identity provider if this account has never been
connected, using the credentials behind the profile the user picked. Say that
plainly first, not as an afterthought once it is done.

Call `provision_cloud_role` with the chosen profile's name and the account
shown beside it in step 2. **Do not ask the user to retype the account** —
they already saw it when they chose the profile, and asking again undoes the
reason it was shown there.

**"Already connected with the same role" is success**, exactly as in the
console-link flow: it means the row already says what you were about to
write, so re-running this is harmless.

**Report the result as registered — never verified or working.** This records
that the control plane has the role on file, not that formae has ever assumed
it successfully.

Then go to step 6.

## Step 4 — "None of these", or no profiles: get the account id

Ask for the 12-digit AWS account id.

**Never infer it** from whatever credentials happen to be lying around.
Provisioning trust into the wrong account is not a mistake a default should be
able to make, which is why the CLI requires it explicitly too.

## Step 5 — Compute the link, hand it over, register

The stack this link points at creates an IAM role the installation can
assume, and, on an account that has never been connected, the account-global
OIDC identity provider that role trusts. The user applies it themselves, in
their own browser, under their own admin session. Nothing here mutates their
cloud: this step computes a link.

Call `connect_cloud_account` with the account id.

Leave `provider_exists` alone unless you have positive reason to set it. A
fresh account has no OIDC provider, that is the dominant case, and the default
creates one. Set it **only** when the user tells you this account was
connected to formae before, in which case the stack creates the role alone.

If the result carries warnings, **show them verbatim**. The multi-installation
warning in particular is telling the user something about their setup that
this skill must not summarize away: several installations managing one cloud
account means duplicate discovery of global resources and two agents
competing to manage them.

Show the user the URL and tell them what applying it creates. Then wait until
they say the stack reached `CREATE_COMPLETE`. Do not run anything on their
behalf while waiting — the work happens in their browser.

Once they confirm, call `register_cloud_role` with the account id and the
expected role ARN from the step above.

**Unless they say the applied role differs.** Quick-create parameters are
editable in the console, and a user who brought their own role has a different
ARN entirely. If they say it differs, ask for the actual ARN from the stack's
outputs and register that.

**"Already connected with the same role" is success.** It means the row
already says what you were about to write, which is what makes re-running this
whole flow harmless. Do not treat it as a conflict and do not try to repair
anything.

**If the stack failed because the provider already exists**, that is the one
case `provider_exists` covers. The recovery is to delete the failed stack,
then run this step again with `provider_exists` set. A stack cannot be
re-applied as an update: a quick-create URL always opens a create flow.

Then go to step 6.

## Step 6 — Confirm, and say how to add more

Tell them the account is connected and name the role.

**Do not mention verification, pending state, or anything to poll.** A
registered connection is complete. There is no verified state in the control
plane, by decision rather than by omission: if the trust is wrong, the first
command that needs it fails loudly and the agent logs say why, which tells
them more than a stamp would and at the moment it actually matters.

Then offer the next step:

- **Another AWS account** on the same installation is supported and is just
  this skill again, from step 1 or step 2. An installation can hold many
  accounts; uniqueness is per installation, cloud, and account.
- **The same account on a second installation** is an anti-pattern, not a
  feature. If they ask, say why: both installations would discover the same
  global resources and compete to manage them. The tool warns about it and
  that warning is the honest answer.
- **Azure or GCP** does not exist yet. Do not imply it is coming.

They can also do the console-link path from a terminal with
`formae connect aws --account <id>`, which walks the same flow interactively.
