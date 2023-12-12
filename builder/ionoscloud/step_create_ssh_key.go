// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ionoscloud

import (
	"context"
	"crypto"
	"encoding/pem"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

type StepCreateSSHKey struct {
	Debug        bool
	DebugKeyPath string
}

func (s *StepCreateSSHKey) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	c := state.Get("config").(*Config)
	ui.Say("Creating ssh key...")

	if c.Comm.SSHPrivateKeyFile != "" {
		pemBytes, err := c.Comm.ReadSSHPrivateKeyFile()
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		priv, pub, err := parsePrivateKey(pemBytes)
		if err != nil {
			state.Put("error", err.Error())
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		c.Comm.SSHPrivateKey = pem.EncodeToMemory(priv)
		c.Comm.SSHPublicKey = ssh.MarshalAuthorizedKey(pub)
	}
	return multistep.ActionContinue
}

func (s *StepCreateSSHKey) Cleanup(state multistep.StateBag) {}

// Attempt to parse the given private key and return the PEM block and public key.
func parsePrivateKey(pem []byte) (*pem.Block, ssh.PublicKey, error) {
	key, err := ssh.ParseRawPrivateKey(pem)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed to parse private key")
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed to create signer")
	}
	pub := signer.PublicKey()
	typed := key.(crypto.PrivateKey)

	block, err := ssh.MarshalPrivateKey(typed, "") //nolint
	return block, pub, nil
}
