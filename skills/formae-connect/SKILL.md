---
name: formae-connect
description: "Use when the user wants to connect a cloud account to their hosted formae installation, or add another one — e.g. 'connect my AWS account', 'give formae access to account 123456789012', 'add a second AWS account', 'formae can't see my resources yet'."
---

# Connect a cloud account

Gives a hosted formae installation permission to manage resources in one AWS
account, by having the user apply a small CloudFormation stack that creates a
role formae can assume.

**Only AWS is supported.** `formae connect` registers an `aws` subcommand and no
other, so there is no Azure or GCP path yet. Say that plainly rather than
offering a choice that dead-ends.

## What the user is agreeing to

The stack creates an IAM role their installation can assume, and, on an account
that has never been connected, the account-global OIDC identity provider that
role trusts. They apply it themselves, in their own browser, under their own
admin session. Nothing here mutates their cloud: this skill computes a link.

## Step 1 — Get the account id

Ask for the 12-digit AWS account id.

**Never infer it** from whatever credentials happen to be lying around. Provisioning
trust into the wrong account is not a mistake a default should be able to make,
which is why the CLI requires it explicitly too.

## Step 2 — Compute the link

Call `connect_cloud_account` with the account id.

Leave `provider_exists` alone unless you have positive reason to set it. A fresh
account has no OIDC provider, that is the dominant case, and the default creates
one. Set it **only** when the user tells you this account was connected to formae
before, in which case the stack creates the role alone.

If the result carries warnings, **show them verbatim**. The multi-installation
warning in particular is telling the user something about their setup that this
skill must not summarize away: several installations managing one cloud account
means duplicate discovery of global resources and two agents competing to manage
them.

## Step 3 — Hand over the link and wait

Show the user the URL and tell them what applying it creates. Then wait until
they say the stack reached `CREATE_COMPLETE`.

Do not run anything on their behalf while waiting. There is no command to run:
the work happens in their browser.

## Step 4 — Register the role

Call `register_cloud_role` with the account id and the expected role ARN from
step 2.

**Unless they say the applied role differs.** Quick-create parameters are
editable in the console, and a user who brought their own role has a different
ARN entirely. If they say it differs, ask for the actual ARN from the stack's
outputs and register that.

**"Already connected with the same role" is success.** It means the row already
says what you were about to write, which is what makes re-running this whole flow
harmless. Do not treat it as a conflict and do not try to repair anything.

**If the stack failed because the provider already exists**, that is the one case
`provider_exists` covers. The recovery is to delete the failed stack, then run
step 2 again with `provider_exists` set. A stack cannot be re-applied as an
update: a quick-create URL always opens a create flow.

## Step 5 — Confirm, and say how to add more

Tell them the account is connected and name the role.

**Do not mention verification, pending state, or anything to poll.** A registered
connection is complete. There is no verified state in the control plane, by
decision rather than by omission: if the trust is wrong, the first command that
needs it fails loudly and the agent logs say why, which tells them more than a
stamp would and at the moment it actually matters.

Then offer the next step:

- **Another AWS account** on the same installation is supported and is just this
  skill again. An installation can hold many accounts; uniqueness is per
  installation, cloud, and account.
- **The same account on a second installation** is an anti-pattern, not a feature.
  If they ask, say why: both installations would discover the same global
  resources and compete to manage them. The tool warns about it and that warning
  is the honest answer.
- **Azure or GCP** does not exist yet. Do not imply it is coming.

They can also do all of this from a terminal with
`formae connect aws --account <id>`, which walks the same flow interactively.
