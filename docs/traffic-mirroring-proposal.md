`# Open vSwitch CNI Plugin - Traffic Mirroring Proposal
`
## Overview

This is a proposal for supporting the traffic mirroring feature of OVS into ovs-cni plugin.
The topic has been initially discussed in [issue 219](https://github.com/k8snetworkplumbingwg/ovs-cni/issues/219).

## Use case

Our project has the requirement of monitoring the network traffic via an IDS/IPS solution included within a dedicated pod. The standard approach is to have an OVS switch capable of mirroring the traffic of specific ports/VLANs into an output SPAN/RSPAN port.

## Goal

Create and manage multiple mirror ports through either ovs-cni or a dedicated cni-plugin in Network Configuration List (Multus) as defined in CNI spec 0.4.0.

## Requirements

- Create multiple mirror ports in a specific bridge
- Select source ports
- Select output port (SPAN)

## API and test-cases

### Premises

1. The approach relies first on the current `ovs` plugins to create the requested port via pod annotation. Afterwards, the output of the plugin execution is cascaded as input to the plugin that is responsible for managing the mirrors  (e.g. `ovs-mirror-producer` and `ovs-mirror-consumer` plugins). This is possible thanks to [Multus chaining capability](https://github.com/containernetworking/cni/blob/spec-v0.4.0/SPEC.md#network-configuration-lists).
2. In all diagrams below we used different colors to represent the logical relation between different entities. In case of OVS they are real DB relations, in case of Pods they represent network connections. Instead, NADs are represented with random colors without a real meaning.
3. In all diagrams below we focused on OVS Mirror `src_port` and `dst_port` to consider the representation with the finest granularity. In this way, we can specify single ports one by one.
However, we are not expert in OVS and its database. In the official [DB Schema 7.10.1 PDF of Open vSwitch 2.3.90](http://www.openvswitch.org//ovs-vswitchd.conf.db.5.pdf) there is a detailed documentation about database relations and columns. Page 41 explains the *Mirror* table, but the *Selecting Packets for Mirroring* paragraph seems a little bit confusing.
Initially we consider only `dst_port` and `src_port`, since with these two options you can build the most fine grained configuration for the mirrors. The only important aspect is to allow the users to set both src and dst at the same time.
For simplicity, we ignore `output_vlan` as mirror output.


### ** NAD approach**

In this approach the complexity is shifted toward the NAD configuration side. The more the scenario is complex (e.g. a lot of vlans, multiple mirrors and different behaviour between pods) the more a greater number of NADs must be defined prior to the pods deployment (could become a little bit messy in complex cases). However, since security is the highest priority, this solution is preferred.

**Producer NAD**

```json
{
    "type": "ovs-mirror-producer",
    "bridge": BRIDGE_NAME,
    "mirrors": [
        {
            "name": MIRROR_NAME,
            "ingress": INGRESS_ENABLED,
            "egress": EGRESS_ENABLED
        },
        (...)
    ]
}
```

`BRIDGE_NAME`: string that represents the unique name of the bridge in ovs database where the mirror should be added

`MIRROR_NAME`: string that represents the unique name of the mirror in ovs database

`INGRESS_ENABLED`: if true it enables ovs mirror src_port

`EGRESS_ENABLED`: if true it enables ovs mirror dst_port


**Consumer NAD**

```json
{
    "type": "ovs-mirror-consumer",
    "bridge": BRIDGE_NAME,
    "mirrors": [
        {
            "name": MIRROR_NAME
        }
    ]
}
```

`BRIDGE_NAME`: string that represents the unique name of the bridge in ovs database where the mirror should be added

`MIRROR_NAME`: string that represents the unique name of the mirror in ovs database


#### **Test case 1**

![ovs-cni-mirror-1A.png](images/ovs-cni-mirror-1A.png)

```yaml
# Produce to 2 mirrors and consume from 1
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan100-prod-mir1-prod-mir2
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 100
            },
            {
                "type": "ovs-mirror-producer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-1",
                        "ingress": true,
                        "egress": true
                    },
                    {
                        "name": "mirror-2",
                        "ingress": true,
                        "egress": true
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan200-cons-mir1
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1"
            },
            {
                "type": "ovs-mirror-consumer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-1"
                    }
                ]
            }
        ]
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-prod-cons
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan100-prod-mir1-prod-mir2",
              "interface":"eth1",
              "namespace":"emu-cni"
            },
            {
              "name":"ovs-vlan200-cons-mir1",
              "interface":"eth2",
              "namespace":"emu-cni"
            }
          ]
```
#### **Test case 2**

![ovs-cni-mirror-2A.png](images/ovs-cni-mirror-2A.png)

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan100-prod-mir-1-prod-mir-2
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 100
            },
            {
                "type": "ovs-mirror-producer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-1",
                        "ingress": true,
                        "egress": true
                    },
                    {
                        "name": "mirror-2",
                        "ingress": true,
                        "egress": true
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan100-prod-mir-2
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 100
            },
            {
                "type": "ovs-mirror-producer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-1",
                        "ingress": true,
                        "egress": true
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-cons-mir-1
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1"
            },
            {
                "type": "ovs-mirror-consumer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-1"
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-cons-mir-2
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1"
            },
            {
                "type": "ovs-mirror-consumer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-2"
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan200
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 200
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan300
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 300
            }
        ]
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-producer-1-vlan100-mir1-mir2
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan100-prod-mir-1-prod-mir-2",
              "interface":"eth1",
              "namespace":"emu-cni"
            }
          ]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-producer-2-vlan100-mir1
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan100-prod-mir-1",
              "interface":"eth1",
              "namespace":"emu-cni"
            }
          ]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-consumer1-mir1
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan200",
              "interface":"eth1",
              "namespace":"emu-cni"
            },
            {
              "name":"ovs-cons-mir-1",
              "interface":"eth2",
              "namespace":"emu-cni"
            }
          ]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-consumer1-mir2
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan300",
              "interface":"eth1",
              "namespace":"emu-cni"
            },
            {
              "name":"ovs-cons-mir-2",
              "interface":"eth2",
              "namespace":"emu-cni"
            }
          ]
```

#### **Test case 3**

![ovs-cni-mirror-3A.png](images/ovs-cni-mirror-3A.png)

```yaml
# Produce to 2 mirrors and consume from 1
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan100-prod-mir1
spec:
  config: |-
    {
       "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 100
            },
            {
                "type": "ovs-mirror-producer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-1",
                        "ingress": true,
                        "egress": true
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan200-prod-mir2
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 200
            },
            {
                "type": "ovs-mirror-producer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-2",
                        "ingress": true,
                        "egress": true
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-vlan300
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1",
                "vlan": 300
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-cons-mir1
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1"
            },
            {
                "type": "ovs-mirror-consumer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-1"
                    }
                ]
            }
        ]
    }
---
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-cons-mir2
spec:
  config: |-
    {
        "cniVersion": "0.4.0",
        "plugins": [
            {
                "type": "ovs",
                "bridge": "br1"
            },
            {
                "type": "ovs-mirror-consumer",
                "bridge": "br1",
                "mirrors": [
                    {
                        "name": "mirror-2"
                    }
                ]
            }
        ]
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-prod-1
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan100-prod-mir1",
              "interface":"eth1",
              "namespace":"emu-cni"
            }
          ]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-prod-2
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan200-prod-mir2",
              "interface":"eth1",
              "namespace":"emu-cni"
            }
          ]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ovs-cons
spec:
  template:
    metadata:
      annotations:
        k8s.v1.cni.cncf.io/networks: |-
          [
            {
              "name":"ovs-vlan300",
              "interface":"eth1",
              "namespace":"emu-cni"
            },
            {
              "name":"ovs-cons-mir1",
              "interface":"eth2",
              "namespace":"emu-cni"
            },
            {
              "name":"ovs-cons-mir2",
              "interface":"eth3",
              "namespace":"emu-cni"
            }
          ]
```


## Questions

- Who is responsible for the mirror creation?
    - both producer and consumer could operate in INSERT OR UPDATE mode
- Who is responsible for the mirror deletion? Would it be viable to simply delete a mirror when it has no other producers or consumers attached to it on the host?
    - if yes, we could use the same strategy of ovs-cni plugin introduced here https://github.com/k8snetworkplumbingwg/ovs-cni/pull/109/files, otherwise we need to implement a new logic
- How does OVSDB handles concurrency? E.g. a producer and a consumer (or 2 producers) try to insert the same row in the `Mirrors` table at the same time
    - OVSDB should offers transaction objects. If we model the whole mirror operation as a single transaction, they should not race.
