(window.webpackJsonp=window.webpackJsonp||[]).push([[80],{643:function(t,e,a){"use strict";a.r(e);var o=a(0),n=Object(o.a)({},(function(){var t=this,e=t.$createElement,a=t._self._c||e;return a("ContentSlotsDistributor",{attrs:{"slot-key":t.$parent.slotKey}},[a("h1",{attrs:{id:"docker-compose"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#docker-compose"}},[t._v("#")]),t._v(" Docker Compose")]),t._v(" "),a("p",[t._v("With Docker Compose, you can spin up local testnets with a single command.")]),t._v(" "),a("h2",{attrs:{id:"requirements"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#requirements"}},[t._v("#")]),t._v(" Requirements")]),t._v(" "),a("ol",[a("li",[a("RouterLink",{attrs:{to:"/introduction/install.html"}},[t._v("Install tendermint")])],1),t._v(" "),a("li",[a("a",{attrs:{href:"https://docs.docker.com/engine/installation/",target:"_blank",rel:"noopener noreferrer"}},[t._v("Install docker"),a("OutboundLink")],1)]),t._v(" "),a("li",[a("a",{attrs:{href:"https://docs.docker.com/compose/install/",target:"_blank",rel:"noopener noreferrer"}},[t._v("Install docker-compose"),a("OutboundLink")],1)])]),t._v(" "),a("h2",{attrs:{id:"build"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#build"}},[t._v("#")]),t._v(" Build")]),t._v(" "),a("p",[t._v("Build the "),a("code",[t._v("tendermint")]),t._v(" binary and, optionally, the "),a("code",[t._v("tendermint/localnode")]),t._v("\ndocker image.")]),t._v(" "),a("p",[t._v("Note the binary will be mounted into the container so it can be updated without\nrebuilding the image.")]),t._v(" "),a("tm-code-block",{staticClass:"codeblock",attrs:{language:"",base64:"Y2QgJEdPUEFUSC9zcmMvZ2l0aHViLmNvbS90ZW5kZXJtaW50L3RlbmRlcm1pbnQKCiMgQnVpbGQgdGhlIGxpbnV4IGJpbmFyeSBpbiAuL2J1aWxkCm1ha2UgYnVpbGQtbGludXgKCiMgKG9wdGlvbmFsbHkpIEJ1aWxkIHRlbmRlcm1pbnQvbG9jYWxub2RlIGltYWdlCm1ha2UgYnVpbGQtZG9ja2VyLWxvY2Fsbm9kZQo="}}),t._v(" "),a("h2",{attrs:{id:"run-a-testnet"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#run-a-testnet"}},[t._v("#")]),t._v(" Run a testnet")]),t._v(" "),a("p",[t._v("To start a 4 node testnet run:")]),t._v(" "),a("tm-code-block",{staticClass:"codeblock",attrs:{language:"",base64:"bWFrZSBsb2NhbG5ldC1zdGFydAo="}}),t._v(" "),a("p",[t._v("The nodes bind their RPC servers to ports 26657, 26660, 26662, and 26664 on the\nhost.")]),t._v(" "),a("p",[t._v("This file creates a 4-node network using the localnode image.")]),t._v(" "),a("p",[t._v("The nodes of the network expose their P2P and RPC endpoints to the host machine\non ports 26656-26657, 26659-26660, 26661-26662, and 26663-26664 respectively.")]),t._v(" "),a("p",[t._v("To update the binary, just rebuild it and restart the nodes:")]),t._v(" "),a("tm-code-block",{staticClass:"codeblock",attrs:{language:"",base64:"bWFrZSBidWlsZC1saW51eAptYWtlIGxvY2FsbmV0LXN0b3AKbWFrZSBsb2NhbG5ldC1zdGFydAo="}}),t._v(" "),a("h2",{attrs:{id:"configuration"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#configuration"}},[t._v("#")]),t._v(" Configuration")]),t._v(" "),a("p",[t._v("The "),a("code",[t._v("make localnet-start")]),t._v(" creates files for a 4-node testnet in "),a("code",[t._v("./build")]),t._v(" by\ncalling the "),a("code",[t._v("tendermint testnet")]),t._v(" command.")]),t._v(" "),a("p",[t._v("The "),a("code",[t._v("./build")]),t._v(" directory is mounted to the "),a("code",[t._v("/tendermint")]),t._v(" mount point to attach\nthe binary and config files to the container.")]),t._v(" "),a("p",[t._v("To change the number of validators / non-validators change the "),a("code",[t._v("localnet-start")]),t._v(" Makefile target:")]),t._v(" "),a("tm-code-block",{staticClass:"codeblock",attrs:{language:"",base64:"bG9jYWxuZXQtc3RhcnQ6IGxvY2FsbmV0LXN0b3AKCUBpZiAhIFsgLWYgYnVpbGQvbm9kZTAvY29uZmlnL2dlbmVzaXMuanNvbiBdOyB0aGVuIGRvY2tlciBydW4gLS1ybSAtdiAkKENVUkRJUikvYnVpbGQ6L3RlbmRlcm1pbnQ6WiB0ZW5kZXJtaW50L2xvY2Fsbm9kZSB0ZXN0bmV0IC0tdiA1IC0tbiAzIC0tbyAuIC0tcG9wdWxhdGUtcGVyc2lzdGVudC1wZWVycyAtLXN0YXJ0aW5nLWlwLWFkZHJlc3MgMTkyLjE2Ny4xMC4yIDsgZmkKCWRvY2tlci1jb21wb3NlIHVwCg=="}}),t._v(" "),a("p",[t._v("The command now will generate config files for 5 validators and 3\nnon-validators network.")]),t._v(" "),a("p",[t._v("Before running it, don't forget to cleanup the old files:")]),t._v(" "),a("tm-code-block",{staticClass:"codeblock",attrs:{language:"",base64:"Y2QgJEdPUEFUSC9zcmMvZ2l0aHViLmNvbS90ZW5kZXJtaW50L3RlbmRlcm1pbnQKCiMgQ2xlYXIgdGhlIGJ1aWxkIGZvbGRlcgpybSAtcmYgLi9idWlsZC9ub2RlKgo="}}),t._v(" "),a("h2",{attrs:{id:"configuring-abci-containers"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#configuring-abci-containers"}},[t._v("#")]),t._v(" Configuring abci containers")]),t._v(" "),a("p",[t._v("To use your own abci applications with 4-node setup edit the "),a("a",{attrs:{href:"https://github.com/tendermint/tendermint/blob/v0.33.x/docker-compose.yml",target:"_blank",rel:"noopener noreferrer"}},[t._v("docker-compose.yaml"),a("OutboundLink")],1),t._v(" file and add image to your abci application.")]),t._v(" "),a("tm-code-block",{staticClass:"codeblock",attrs:{language:"",base64:"IGFiY2kwOgogICAgY29udGFpbmVyX25hbWU6IGFiY2kwCiAgICBpbWFnZTogJnF1b3Q7YWJjaS1pbWFnZSZxdW90OwogICAgYnVpbGQ6CiAgICAgIGNvbnRleHQ6IC4KICAgICAgZG9ja2VyZmlsZTogYWJjaS5Eb2NrZXJmaWxlCiAgICBjb21tYW5kOiAmbHQ7aW5zZXJ0IGNvbW1hbmQgdG8gcnVuIHlvdXIgYWJjaSBhcHBsaWNhdGlvbiZndDsKICAgIG5ldHdvcmtzOgogICAgICBsb2NhbG5ldDoKICAgICAgICBpcHY0X2FkZHJlc3M6IDE5Mi4xNjcuMTAuNgoKICBhYmNpMToKICAgIGNvbnRhaW5lcl9uYW1lOiBhYmNpMQogICAgaW1hZ2U6ICZxdW90O2FiY2ktaW1hZ2UmcXVvdDsKICAgIGJ1aWxkOgogICAgICBjb250ZXh0OiAuCiAgICAgIGRvY2tlcmZpbGU6IGFiY2kuRG9ja2VyZmlsZQogICAgY29tbWFuZDogJmx0O2luc2VydCBjb21tYW5kIHRvIHJ1biB5b3VyIGFiY2kgYXBwbGljYXRpb24mZ3Q7CiAgICBuZXR3b3JrczoKICAgICAgbG9jYWxuZXQ6CiAgICAgICAgaXB2NF9hZGRyZXNzOiAxOTIuMTY3LjEwLjcKCiAgYWJjaTI6CiAgICBjb250YWluZXJfbmFtZTogYWJjaTIKICAgIGltYWdlOiAmcXVvdDthYmNpLWltYWdlJnF1b3Q7CiAgICBidWlsZDoKICAgICAgY29udGV4dDogLgogICAgICBkb2NrZXJmaWxlOiBhYmNpLkRvY2tlcmZpbGUKICAgIGNvbW1hbmQ6ICZsdDtpbnNlcnQgY29tbWFuZCB0byBydW4geW91ciBhYmNpIGFwcGxpY2F0aW9uJmd0OwogICAgbmV0d29ya3M6CiAgICAgIGxvY2FsbmV0OgogICAgICAgIGlwdjRfYWRkcmVzczogMTkyLjE2Ny4xMC44CgogIGFiY2kzOgogICAgY29udGFpbmVyX25hbWU6IGFiY2kzCiAgICBpbWFnZTogJnF1b3Q7YWJjaS1pbWFnZSZxdW90OwogICAgYnVpbGQ6CiAgICAgIGNvbnRleHQ6IC4KICAgICAgZG9ja2VyZmlsZTogYWJjaS5Eb2NrZXJmaWxlCiAgICBjb21tYW5kOiAmbHQ7aW5zZXJ0IGNvbW1hbmQgdG8gcnVuIHlvdXIgYWJjaSBhcHBsaWNhdGlvbiZndDsKICAgIG5ldHdvcmtzOgogICAgICBsb2NhbG5ldDoKICAgICAgICBpcHY0X2FkZHJlc3M6IDE5Mi4xNjcuMTAuOQoK"}}),t._v(" "),a("p",[t._v("Override the "),a("a",{attrs:{href:"https://github.com/tendermint/tendermint/blob/v0.33.x/networks/local/localnode/Dockerfile#L12",target:"_blank",rel:"noopener noreferrer"}},[t._v("command"),a("OutboundLink")],1),t._v(" in each node to connect to it's abci.")]),t._v(" "),a("tm-code-block",{staticClass:"codeblock",attrs:{language:"",base64:"ICBub2RlMDoKICAgIGNvbnRhaW5lcl9uYW1lOiBub2RlMAogICAgaW1hZ2U6ICZxdW90O3RlbmRlcm1pbnQvbG9jYWxub2RlJnF1b3Q7CiAgICBwb3J0czoKICAgICAgLSAmcXVvdDsyNjY1Ni0yNjY1NzoyNjY1Ni0yNjY1NyZxdW90OwogICAgZW52aXJvbm1lbnQ6CiAgICAgIC0gSUQ9MAogICAgICAtIExPRz0kJHtMT0c6LXRlbmRlcm1pbnQubG9nfQogICAgdm9sdW1lczoKICAgICAgLSAuL2J1aWxkOi90ZW5kZXJtaW50OloKICAgIGNvbW1hbmQ6IG5vZGUgLS1wcm94eV9hcHA9dGNwOi8vYWJjaTA6MjY2NTgKICAgIG5ldHdvcmtzOgogICAgICBsb2NhbG5ldDoKICAgICAgICBpcHY0X2FkZHJlc3M6IDE5Mi4xNjcuMTAuMgo="}}),t._v(" "),a("p",[t._v("Similarly do for node1, node2 and node3 then "),a("a",{attrs:{href:"https://github.com/tendermint/tendermint/blob/v0.33.x/docs/networks/docker-compose.md#run-a-testnet",target:"_blank",rel:"noopener noreferrer"}},[t._v("run testnet"),a("OutboundLink")],1)]),t._v(" "),a("h2",{attrs:{id:"logging"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#logging"}},[t._v("#")]),t._v(" Logging")]),t._v(" "),a("p",[t._v("Log is saved under the attached volume, in the "),a("code",[t._v("tendermint.log")]),t._v(" file. If the\n"),a("code",[t._v("LOG")]),t._v(" environment variable is set to "),a("code",[t._v("stdout")]),t._v(" at start, the log is not saved,\nbut printed on the screen.")]),t._v(" "),a("h2",{attrs:{id:"special-binaries"}},[a("a",{staticClass:"header-anchor",attrs:{href:"#special-binaries"}},[t._v("#")]),t._v(" Special binaries")]),t._v(" "),a("p",[t._v("If you have multiple binaries with different names, you can specify which one\nto run with the "),a("code",[t._v("BINARY")]),t._v(" environment variable. The path of the binary is relative\nto the attached volume.")])],1)}),[],!1,null,null,null);e.default=n.exports}}]);