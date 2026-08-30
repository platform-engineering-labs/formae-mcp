package server

import "testing"

// This file pins the exact bytes renderRegistered, renderGcpRegistered, and
// renderAzureRegistered produce today. The three functions are about to be
// collapsed into one parameterised renderer; a person has already read this
// wording after connecting an AWS or GCP account, so nothing here may move by
// so much as a space. Each case below was captured from the pre-refactor
// source and must keep passing, unchanged, once the internals are shared.
func TestRenderRegistered_AWS_FirstTime(t *testing.T) {
	d := registeredDoc{
		Status:  "registered_unverified",
		Account: "123456789012",
		RoleArn: "arn:aws:iam::123456789012:role/formae-connect-2abc",
	}
	want := "Connected account 123456789012.\n" +
		"Role: arn:aws:iam::123456789012:role/formae-connect-2abc\n"
	if got := renderRegistered(d); got != want {
		t.Errorf("renderRegistered mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRenderRegistered_AWS_AlreadyRegisteredWithWarnings(t *testing.T) {
	d := registeredDoc{
		Status:   statusAlreadyRegistered,
		Account:  "123456789012",
		RoleArn:  "arn:aws:iam::123456789012:role/formae-connect-2abc",
		Warnings: []string{"w1", "w2"},
	}
	want := "Account 123456789012 was already connected to this installation with the same role.\n" +
		"Role: arn:aws:iam::123456789012:role/formae-connect-2abc\n" +
		"\nWarning: w1\n" +
		"\nWarning: w2\n"
	if got := renderRegistered(d); got != want {
		t.Errorf("renderRegistered mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRenderGcpRegistered_FirstTime(t *testing.T) {
	d := registeredDoc{
		Status:                   "registered_unverified",
		Account:                  "example-project",
		WorkloadIdentityProvider: testGcpProvider,
	}
	want := "Connected project example-project.\n" +
		"Workload identity provider: " + testGcpProvider + "\n"
	if got := renderGcpRegistered(d); got != want {
		t.Errorf("renderGcpRegistered mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRenderGcpRegistered_AlreadyRegisteredWithWarning(t *testing.T) {
	d := registeredDoc{
		Status:                   statusAlreadyRegistered,
		Account:                  "example-project",
		WorkloadIdentityProvider: testGcpProvider,
		Warnings:                 []string{"gcp warn"},
	}
	want := "Project example-project was already connected to this installation with the same federation.\n" +
		"Workload identity provider: " + testGcpProvider + "\n" +
		"\nWarning: gcp warn\n"
	if got := renderGcpRegistered(d); got != want {
		t.Errorf("renderGcpRegistered mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRenderAzureRegistered_FirstTime(t *testing.T) {
	d := registeredDoc{
		Status:        "registered_unverified",
		Account:       testAzureSubscription,
		AzureTenantID: testAzureTenant,
		AzureClientID: testAzureClient,
	}
	want := "Connected subscription " + testAzureSubscription + ".\n" +
		"Tenant: " + testAzureTenant + "\n" +
		"Client id: " + testAzureClient + "\n"
	if got := renderAzureRegistered(d); got != want {
		t.Errorf("renderAzureRegistered mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRenderAzureRegistered_AlreadyRegisteredWithWarning(t *testing.T) {
	d := registeredDoc{
		Status:        statusAlreadyRegistered,
		Account:       testAzureSubscription,
		AzureTenantID: testAzureTenant,
		AzureClientID: testAzureClient,
		Warnings:      []string{"azure warn"},
	}
	want := "Subscription " + testAzureSubscription + " was already connected to this installation with the same identity.\n" +
		"Tenant: " + testAzureTenant + "\n" +
		"Client id: " + testAzureClient + "\n" +
		"\nWarning: azure warn\n"
	if got := renderAzureRegistered(d); got != want {
		t.Errorf("renderAzureRegistered mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}
