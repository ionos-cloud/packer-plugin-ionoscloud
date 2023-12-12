<!--
  Include a short overview about the plugin.

  This document is a great location for creating a table of contents for each
  of the components the plugin may provide. This document should load automatically
  when navigating to the docs directory for a plugin.

-->

### Installation

To install this plugin, copy and paste this code into your Packer configuration, then run [`packer init`](https://www.packer.io/docs/commands/init).

```hcl
packer {
  required_plugins {
    ionoscloud = {
      version = ">= 1.0.0"
      source  = "github.com/ionos-cloud/ionoscloud"
    }
  }
}
```

Alternatively, you can use `packer plugins install` to manage installation of this plugin.

```sh
$ packer plugins install github.com/ionos-cloud/ionoscloud
```

### Components

The Scaffolding plugin is intended as a starting point for creating Packer plugins

#### Builders

- [ionoscloud](/packer/integrations/hashicorp/ionoscloud/latest/components/builder/ionoscloud) - The IONOSCloud Builder
  is able to create virtual machines for [IONOS Compute Engine](https://cloud.ionos.com/compute).
