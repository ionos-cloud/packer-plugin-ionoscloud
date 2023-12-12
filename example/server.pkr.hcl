source "ionoscloud" "ubuntu" {
  image                 = "Ubuntu-22.04"
  disk_size             = 5
  snapshot_name         = "test-snapshot"
  ssh_username          = "root"
  ssh_password          = "test1234"
  ssh_timeout          = "15m"
  ssh_private_key_file  = "~/.ssh/id_rsa"

}

build {
  sources = ["ionoscloud.ubuntu"]
}
