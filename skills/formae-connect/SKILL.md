---
name: formae-connect
description: "Use when the user wants to connect a cloud account to their hosted formae installation, or add another one — e.g. 'connect my AWS account', 'give formae access to account 123456789012', 'add a second AWS account', 'formae can't see my resources yet'."
---

# Connect a cloud account

Gives a hosted formae installation permission to manage resources in a cloud
account. AWS has two ways to get there: provisioning directly from local AWS
credentials in one step, or having the user apply a small CloudFormation stack
themselves. GCP and Azure each have one, because neither has a console
equivalent to the CloudFormation quick-create URL to offer.

## Step 1 — Which cloud?

Ask AWS, Azure, or GCP.

**AWS:** go to step 2.

**GCP:** go to step 2G.

**Azure:** go to step 2A.

## Step 2G — GCP: one call

GCP has no console path. CloudFormation's quick-create URL is what makes the
AWS link path possible, and GCP retired its only console-deployable template
service, so there is nothing to hand the user. Do not go looking for an
equivalent and do not offer one.

Ask for the **project id**. Never infer it from gcloud's active configuration:
provisioning trust into the wrong project is not a mistake a default should be
able to make.

Then say what is about to happen, before calling anything:

- formae will create a workload identity pool and provider in that project and
  grant itself access to it. The access is broad — editor, plus the ability to
  manage the project's IAM. Say that in those words; it is what the agent needs
  to manage arbitrary infrastructure, and the user is entitled to know before
  it happens rather than after.
- If the machine has no usable Google credentials, formae signs them in by
  running `gcloud auth application-default login`, which opens a browser. Warn
  them so the browser is expected rather than alarming. If gcloud is not
  installed the call fails saying so, and they will need to install it.

Then call `connect_gcp_project` with the project id. It provisions and
registers in one call; there is no second step to wait for.

**Only if the user says they already set up the federation themselves** — with
Terraform, say, because they will not give a CLI provisioning rights — pass
`workload_identity_provider` as well. That path validates the name's shape and
nothing else: it does not check the provider exists, trusts the formae issuer,
or grants access. The tool's own warning says so; pass it on rather than
softening it.

Then go to step 6: registration is already complete, in this one call, and
steps 3-5 are AWS's own continuation.

## Step 2A — Azure: one call

Azure has no console path either, for the same reason as GCP: there is
nothing like CloudFormation's quick-create URL to hand the user, so
provisioning and registering are one call.

Ask for the **subscription id**. Never infer it from az's active
configuration: provisioning trust into the wrong subscription is not a
mistake a default should be able to make.

Then say what is about to happen, before calling anything:

- formae will create a managed identity in that subscription and grant it
  Contributor plus User Access Administrator — near-owner access. Say that in
  those words; it is what the agent needs to manage arbitrary infrastructure,
  and the user is entitled to know before it happens rather than after.
- formae authenticates with the operator's own ambient Azure credentials
  (environment variables, managed identity, an existing `az login` session)
  and never opens a browser or spawns a sign-in itself. If there are none, the
  call fails naming the exact `az login` command to run — show it to the user
  verbatim, wait for them to run it themselves in their own terminal, then
  call the tool again. Do not run it for them and do not offer to.

Then call `connect_azure_subscription` with the subscription id.

**If the user will not give the agent provisioning credentials at all**, this
tool is not the path: there is no register-only argument on it. They deploy
the embedded ARM template themselves — `formae connect azure template` prints
it, to run in their own console or pipeline, under their own admin session —
and it establishes the trust without formae ever holding a provisioning
credential. Once applied, it prints a tenant id and a client id; they run
`formae connect azure --subscription <id> --tenant-id <id> --client-id <id>`
themselves in their own terminal to register those. **That command is for the
user, never for you** — same as the terminal commands named in step 7. This
path validates the coordinate's shape and nothing else: it does not check the
identity exists, trusts the formae issuer, or grants access, and the CLI's own
warning says so.

Then go to step 6: registration is already complete, in this one call, and
steps 3-5 are AWS's own continuation.

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

## Step 6 — Offer to create a target

Registering an account grants trust and, on its own, leaves the user with
nothing to do: the agent cannot discover or manage anything until a
**target** exists — an account paired with a region, the thing discovery and
every apply both run against. Explain that in a sentence or two, then ask
whether to create one now.

This step writes a forma and applies it, and that is worth saying out loud to
someone seeing formae for the first time, because it is not a detour: in formae
everything is created by declaring it and applying — resources, stacks, targets,
policies alike. There is no verb that creates a target, in the same way there is
no verb that creates a bucket. So the first thing this journey produces is a
small project the user owns, and the target is the first thing declared in it.

**If they decline, go straight to step 7.**

**AWS:** ask for a **region** and a **label** for the target.

**GCP:** ask for a **region** and a **label** too. The target config differs —
it names the project and authenticates through the workload identity provider
rather than a role — but the questions are the same.

**Azure:** ask for a **label** only. There is no region to ask for: an Azure
target names the subscription and authenticates through the tenant and client
id from the registration, and the config carries no region field the way AWS's
and GCP's do.

Ask for the directory to create the project in, proposing the current
directory by name as the default — the same thing `git init`, `npm init`,
and `cargo init` do.

**Unless the current directory is `/` or the user's home directory itself.**
Neither is a place to scatter a project's three files, and a harness that
started in one chose it for the user rather than the user choosing it — which
is what separates this from `git init`, where they cd'd there and typed the
command. Propose `~/<label>` in that case, reusing the target label they gave
a moment ago rather than asking a second naming question. They can still name
any path instead.

**If that directory already contains a `PklProject`, say so and stop.** This
flow creates a fresh project; it does not merge into an existing one. Then go
to step 7.

Otherwise, write three files with the harness's own file tools.

`PklProject`, pinning `formae` and the cloud's plugin schema by canonical
semver. **AWS:**

```pkl
amends "pkl:Project"

dependencies {
  ["formae"] {
    uri = "package://hub.platform.engineering/plugins/pkl/schema/pkl/formae/formae@0.89.0"
  }
  ["aws"] {
    uri = "package://hub.platform.engineering/plugins/aws/schema/pkl/aws/aws@0.1.17"
  }
}
```

**GCP** is the same file with the gcp plugin in place of aws:

```pkl
  ["gcp"] {
    uri = "package://hub.platform.engineering/plugins/gcp/schema/pkl/gcp/gcp@0.1.13"
  }
```

**Azure** is the same file with the azure plugin in place of aws:

```pkl
  ["azure"] {
    uri = "package://hub.platform.engineering/plugins/azure/schema/pkl/azure/azure@0.1.10"
  }
```

**Never pin a `-dev.N` suffix.** A dev build overwrites the schema published
at its canonical coordinate; the coordinates above are the ones that
actually exist to resolve.

`vars.pkl`, holding the definition. **The imports are not optional** —
without them `formae.Target` and the plugin's `Config` and `OidcAuth` are
unresolved and evaluation fails. **AWS:**

```pkl
import "@formae/formae.pkl"
import "@aws/aws.pkl"

awsTarget: formae.Target = new formae.Target {
  label = "<the label they gave>"
  discoverable = true
  config = new aws.Config {
    region = "<the region they gave>"
    auth = new aws.OidcAuth { roleArn = "<the role ARN from the registration>" }
  }
}
```

**GCP** declares the same target with the project and the provider in place of
region and role:

```pkl
import "@formae/formae.pkl"
import "@gcp/gcp.pkl"

gcpTarget: formae.Target = new formae.Target {
  label = "<the label they gave>"
  discoverable = true
  config = new gcp.Config {
    project = "<the project id they gave>"
    region = "<the region they gave>"
    auth = new gcp.OidcAuth {
      workloadIdentityProvider = "<from the registration>"
    }
  }
}
```

**Azure** declares the same target with the subscription and the
registration's tenant and client id in place of region and role. There is no
region field:

```pkl
import "@formae/formae.pkl"
import "@azure/azure.pkl"

azureTarget: formae.Target = new formae.Target {
  label = "<the label they gave>"
  discoverable = true
  config = new azure.Config {
    subscriptionId = "<the subscription id they gave in step 2A>"
    auth = new azure.OidcAuth {
      tenantId = "<from the registration>"
      clientId = "<from the registration>"
    }
  }
}
```

**`discoverable` defaults to `false`.** Leave it off and the target exists,
discovers nothing, and the user comes back later to an empty inventory with
no error to explain it. Set it to `true`, and say why when you show them
this file.

**The trust coordinate comes from the registration, not the user.** Whichever
path reached here — `provision_cloud_role`, `register_cloud_role`,
`connect_gcp_project`, or `connect_azure_subscription` — already returned it,
as a role ARN for AWS, a workload identity provider for GCP, or a tenant and
client id for Azure. Asking the user to retype it invites a typo the target
would silently carry, the same reasoning that governs the account id on the
local-credentials path in step 3. It is long and easy to mistype, which makes
copying it rather than retyping it worth insisting on.

`targets.pkl`, the file that gets applied:

```pkl
amends "@formae/forma.pkl"
import "vars.pkl"

forma {
  vars.awsTarget
}
```

For GCP, spread `vars.gcpTarget` instead; for Azure, `vars.azureTarget`.

Apply `targets.pkl` the way `/formae:apply` does, because this is the user's
first apply and it should look like every one after it: call `apply_forma` with
`mode: reconcile` and `simulate: true`, show them what it will do, then apply
for real.

Simulating matters more here than the tiny forma suggests. Reconcile destroys
deployed resources that the file does not declare, so a forma holding only a
target is a claim about everything in scope, not just about the target. On a
fresh installation there is nothing to destroy and the simulation says so in one
line, which is exactly the point: the user sees the shape of an apply while the
stakes are zero.

**Do not add a separate "resolve the project" step:** formae's evaluation
already runs `pkl project resolve` when `PklProject.deps.json` is absent,
including the evaluation `apply_forma` performs itself.

Then go to step 7.

## Step 7 — Confirm, and say how to add more

Tell them the account is connected and name the trust coordinate that came
back — a role for AWS, a workload identity provider for GCP, or a tenant and
client id for Azure.

**Do not mention verification, pending state, or anything to poll.** A
registered connection is complete. There is no verified state in the control
plane, by decision rather than by omission: if the trust is wrong, the first
command that needs it fails loudly and the agent logs say why, which tells
them more than a stamp would and at the moment it actually matters.

**If a target was created in step 6**, tell them what happens next and give
one example prompt for each:

- Discovery is already running against it. They can come back shortly and
  ask what it found — e.g. `"What unmanaged resources do you see in
  <label>?"`
- They can start creating infrastructure against it now — e.g. `"Create an
  S3 bucket in <label>."`

If no target was created, skip the above — this step needs nothing extra for
that case.

Then offer the next step:

- **Another AWS account** on the same installation is supported and is just
  this skill again, from step 1 or step 2. An installation can hold many
  accounts; uniqueness is per installation, cloud, and account.
- **The same account on a second installation** is an anti-pattern, not a
  feature. If they ask, say why: both installations would discover the same
  global resources and compete to manage them. The tool warns about it and
  that warning is the honest answer.
- **Another Azure subscription** on the same installation is supported the
  same way as another AWS account.
- **The same GCP project, or the same Azure subscription, on a second
  installation** is worth one extra sentence beyond the AWS answer above:
  installations connected to one project or subscription share a trust
  domain, because each is granted enough access to rewrite its IAM, including
  the other's. The tool warns about it; that warning is the honest answer.

They can also do this from a terminal: `formae connect aws --account <id>` for
the console-link path, `formae connect gcp --project <id>` for GCP, or
`formae connect azure --subscription <id>` for Azure. All three walk the same
flow interactively — and for Azure, `formae connect azure --subscription <id>
--tenant-id ... --client-id ...` is the credential-less path named in step 2A.

**That command is for the user, never for you.** Do not run it, and do not
offer to. It is interactive and it provisions real cloud trust; run from a
harness it cannot drive, it burns state and leaves a half-finished flow behind.
Everything this skill needs is available through its tools.
