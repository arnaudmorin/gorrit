package ui

import "testing"

func TestRewriteBareTerms(t *testing.T) {
	cases := map[string]string{
		`dhcp meter`:                             `subject:dhcp subject:meter`,
		`is:open project:openstack/neutron dhcp`: `is:open project:openstack/neutron subject:dhcp`,
		`"dvr arp" owner:self`:                   `subject:"dvr arp" owner:self`,
		`(owner:self OR reviewer:self) fix typo`: `( owner:self OR reviewer:self ) subject:fix subject:typo`,
		`-is:wip meter -owner:bob`:               `-is:wip subject:meter -owner:bob`,
		`subject:"already" message:done`:         `subject:"already" message:done`,
		`topic:"my topic" word`:                  `topic:"my topic" subject:word`,
		``:                                       ``,
	}
	for in, want := range cases {
		if got := rewriteBareTerms(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
