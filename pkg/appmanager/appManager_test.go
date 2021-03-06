/*-
 * Copyright (c) 2016,2017, F5 Networks, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package appmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/F5Networks/k8s-bigip-ctlr/pkg/test"

	routeapi "github.com/openshift/origin/pkg/route/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
)

func init() {
	workingDir, _ := os.Getwd()
	schemaUrl = "file://" + workingDir + "/../../schemas/bigip-virtual-server_v0.1.4.json"
	DEFAULT_PARTITION = "velcro"
}

var schemaUrl string

var configmapFoo string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "foo",
      "servicePort": 80,
      "healthMonitors": [ {
        "interval": 30,
        "timeout": 20,
        "send": "GET /",
        "protocol": "tcp"
        }
      ]
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "http",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 5051
      },
      "sslProfile": {
        "f5ProfileName": "velcro/testcert"
      }
    }
  }
}`)

var configmapFoo8080 string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "foo",
      "servicePort": 8080
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "http",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 5051
      }
    }
  }
}`)

var configmapFoo9090 string = string(`{
	"virtualServer": {
		"backend": {
			"serviceName": "foo",
			"servicePort": 9090
		},
		"frontend": {
			"balance": "round-robin",
			"mode": "tcp",
			"partition": "velcro",
			"virtualAddress": {
				"bindAddr": "10.128.10.200",
				"port": 4041
			}
		}
	}
}`)

var configmapFooTcp string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "foo",
      "servicePort": 80
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "tcp",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 5051
      }
    }
  }
}`)

var configmapFooInvalid string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "",
      "servicePort": 0
    },
    "frontend": {
      "balance": "super-duper-mojo",
      "mode": "udp",
      "partition": "",
      "virtualAddress": {
        "bindAddr": "10.128.10.260",
        "port": 500000
      },
      "sslProfile": {
        "f5ProfileName": ""
      }
    }
  }
}`)

var configmapBar string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "bar",
      "servicePort": 80
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "http",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 6051
      }
    }
  }
}`)

var configmapNoModeBalance string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "bar",
      "servicePort": 80
    },
    "frontend": {
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 80
      }
    }
  }
}`)

var configmapIApp1 string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "iapp1",
      "servicePort": 80
    },
    "frontend": {
      "partition": "velcro",
      "iapp": "/Common/f5.http",
      "iappPoolMemberTable": {
        "name": "pool__members",
        "columns": [
          {"name": "IPAddress", "kind": "IPAddress"},
          {"name": "Port", "kind": "Port"},
          {"name": "ConnectionLimit", "value": "0"},
          {"name": "SomeOtherValue", "value": "value-1"}
        ]
      },
      "iappOptions": {
        "description": "iApp 1"
      },
      "iappVariables": {
        "monitor__monitor": "/#create_new#",
        "monitor__resposne": "none",
        "monitor__uri": "/",
        "net__client_mode": "wan",
        "net__server_mode": "lan",
        "pool__addr": "127.0.0.1",
        "pool__pool_to_use": "/#create_new#",
        "pool__port": "8080"
      }
    }
  }
}`)

var configmapIApp2 string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "iapp2",
      "servicePort": 80
    },
    "frontend": {
      "partition": "velcro",
      "iapp": "/Common/f5.http",
      "iappOptions": {
        "description": "iApp 2"
      },
      "iappTables": {
        "pool__Pools": {
          "columns": ["Index", "Name", "Description", "LbMethod", "Monitor",
                      "AdvOptions"],
          "rows": [["0", "", "", "round-robin", "0", "none"]]
        },
        "monitor__Monitors": {
          "columns": ["Index", "Name", "Type", "Options"],
          "rows": [["0", "/Common/tcp", "none", "none"]]
        }
      },
      "iappPoolMemberTable": {
        "name": "pool__members",
        "columns": [
          {"name": "IPAddress", "kind": "IPAddress"},
          {"name": "Port", "kind": "Port"},
          {"name": "ConnectionLimit", "value": "0"},
          {"name": "SomeOtherValue", "value": "value-1"}
        ]
      },
      "iappVariables": {
        "monitor__monitor": "/#create_new#",
        "monitor__resposne": "none",
        "monitor__uri": "/",
        "net__client_mode": "wan",
        "net__server_mode": "lan",
        "pool__addr": "127.0.0.2",
        "pool__pool_to_use": "/#create_new#",
        "pool__port": "4430"
      }
    }
  }
}`)

var emptyConfig string = string(`{"resources":{}}`)

var twoSvcsFourPortsThreeNodesConfig string = string(`{"resources":{"virtualServers":[{"name":"default_barmap","pool":"/velcro/default_barmap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}},{"name":"default_foomap","pool":"/velcro/default_foomap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051},"sslProfile":{"f5ProfileName":"velcro/testcert"}},{"name":"default_foomap8080","pool":"/velcro/default_foomap8080","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051}},{"name":"default_foomap9090","pool":"/velcro/default_foomap9090","partition":"velcro","mode":"tcp","virtualAddress":{"bindAddr":"10.128.10.200","port":4041}}],"pools":[{"name":"default_barmap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"bar","servicePort":80,"poolMemberAddrs":["127.0.0.1:37001","127.0.0.2:37001","127.0.0.3:37001"],"monitor":null},{"name":"default_foomap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"foo","servicePort":80,"poolMemberAddrs":["127.0.0.1:30001","127.0.0.2:30001","127.0.0.3:30001"],"monitor":["/velcro/default_foomap"]},{"name":"default_foomap8080","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"foo","servicePort":8080,"poolMemberAddrs":["127.0.0.1:38001","127.0.0.2:38001","127.0.0.3:38001"],"monitor":null},{"name":"default_foomap9090","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"foo","servicePort":9090,"poolMemberAddrs":["127.0.0.1:39001","127.0.0.2:39001","127.0.0.3:39001"],"monitor":null}],"monitors":[{"name":"default_foomap","partition":"velcro","interval":30,"protocol":"tcp","send":"GET /","timeout":20}]}}`)

var twoSvcsTwoNodesConfig string = string(`{"resources":{"virtualServers":[{"name":"default_barmap","pool":"/velcro/default_barmap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}},{"name":"default_foomap","pool":"/velcro/default_foomap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051},"sslProfile":{"f5ProfileName":"velcro/testcert"}}],"pools":[{"name":"default_barmap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"bar","servicePort":80,"poolMemberAddrs":["127.0.0.1:37001","127.0.0.2:37001"]},{"name":"default_foomap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"foo","servicePort":80,"poolMemberAddrs":["127.0.0.1:30001","127.0.0.2:30001"],"monitor":["/velcro/default_foomap"]}],"monitors":[{"name":"default_foomap","partition":"velcro","interval":30,"protocol":"tcp","send":"GET /","timeout":20}]}}`)

var twoSvcsOneNodeConfig string = string(`{"resources":{"virtualServers":[{"name":"default_barmap","pool":"/velcro/default_barmap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}},{"name":"default_foomap","pool":"/velcro/default_foomap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051},"sslProfile":{"f5ProfileName":"velcro/testcert"}}],"pools":[{"name":"default_barmap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"bar","servicePort":80,"poolMemberAddrs":["127.0.0.3:37001"]},{"name":"default_foomap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"foo","servicePort":80,"poolMemberAddrs":["127.0.0.3:30001"],"monitor":["/velcro/default_foomap"]}],"monitors":[{"name":"default_foomap","partition":"velcro","interval":30,"protocol":"tcp","send":"GET /","timeout":20}]}}`)

var oneSvcOneNodeConfig string = string(`{"resources":{"virtualServers":[{"name":"default_barmap","pool":"/velcro/default_barmap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}],"pools":[{"name":"default_barmap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"bar","servicePort":80,"poolMemberAddrs":["127.0.0.3:37001"]}]}}`)

var twoIappsThreeNodesConfig string = string(`{"resources":{"virtualServers":[{"name":"default_iapp1map","pool":"/velcro/default_iapp1map","partition":"velcro","mode":"tcp","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 1"},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.1","pool__pool_to_use":"/#create_new#","pool__port":"8080"}},{"name":"default_iapp2map","pool":"/velcro/default_iapp2map","partition":"velcro","mode":"tcp","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 2"},"iappTables":{"pool__Pools":{"columns":["Index","Name","Description","LbMethod","Monitor","AdvOptions"],"rows":[["0","","","round-robin","0","none"]]},"monitor__Monitors":{"columns":["Index","Name","Type","Options"],"rows":[["0","/Common/tcp","none","none"]]}},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.2","pool__pool_to_use":"/#create_new#","pool__port":"4430"}}],"pools":[{"name":"default_iapp1map","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"iapp1","servicePort":80,"poolMemberAddrs":["192.168.0.1:10101","192.168.0.2:10101","192.168.0.4:10101"],"monitor":null},{"name":"default_iapp2map","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"iapp2","servicePort":80,"poolMemberAddrs":["192.168.0.1:20202","192.168.0.2:20202","192.168.0.4:20202"],"monitor":null}]}}`)

var twoIappsOneNodeConfig string = string(`{"resources":{"virtualServers":[{"name":"default_iapp1map","pool":"/velcro/default_iapp1map","partition":"velcro","mode":"tcp","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 1"},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.1","pool__pool_to_use":"/#create_new#","pool__port":"8080"}},{"name":"default_iapp2map","pool":"/velcro/default_iapp2map","partition":"velcro","mode":"tcp","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 2"},"iappTables":{"pool__Pools":{"columns":["Index","Name","Description","LbMethod","Monitor","AdvOptions"],"rows":[["0","","","round-robin","0","none"]]},"monitor__Monitors":{"columns":["Index","Name","Type","Options"],"rows":[["0","/Common/tcp","none","none"]]}},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.2","pool__pool_to_use":"/#create_new#","pool__port":"4430"}}],"pools":[{"name":"default_iapp1map","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"iapp1","servicePort":80,"poolMemberAddrs":["192.168.0.4:10101"],"monitor":null},{"name":"default_iapp2map","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"iapp2","servicePort":80,"poolMemberAddrs":["192.168.0.4:20202"],"monitor":null}]}}`)

var oneIappOneNodeConfig string = string(`{"resources":{"virtualServers":[{"name":"default_iapp2map","pool":"/velcro/default_iapp2map","partition":"velcro","mode":"tcp","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 2"},"iappTables":{"pool__Pools":{"columns":["Index","Name","Description","LbMethod","Monitor","AdvOptions"],"rows":[["0","","","round-robin","0","none"]]},"monitor__Monitors":{"columns":["Index","Name","Type","Options"],"rows":[["0","/Common/tcp","none","none"]]}},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.2","pool__pool_to_use":"/#create_new#","pool__port":"4430"}}],"pools":[{"name":"default_iapp2map","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"iapp2","servicePort":80,"poolMemberAddrs":["192.168.0.4:20202"],"monitor":null}]}}`)

var twoSvcTwoPodsConfig string = string(`{"resources":{"virtualServers":[{"name":"default_barmap","pool":"/velcro/default_barmap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}},{"name":"default_foomap","pool":"/velcro/default_foomap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051}}],"pools":[{"name":"default_barmap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"bar","servicePort":80,"poolMemberAddrs":["10.2.96.0:80","10.2.96.3:80"]},{"name":"default_foomap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"foo","servicePort":8080,"poolMemberAddrs":["10.2.96.1:8080","10.2.96.2:8080"]}]}}`)

var oneSvcTwoPodsConfig string = string(`{"resources":{"virtualServers":[{"name":"default_barmap","pool":"/velcro/default_barmap","partition":"velcro","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}],"pools":[{"name":"default_barmap","partition":"velcro","loadBalancingMode":"round-robin","serviceName":"bar","servicePort":80,"poolMemberAddrs":["10.2.96.0:80","10.2.96.3:80"]}]}}`)

type mockAppManager struct {
	appMgr  *Manager
	mutex   sync.Mutex
	vsMutex map[serviceQueueKey]*sync.Mutex
	nsLabel string
}

func newMockAppManager(params *Params) *mockAppManager {
	return &mockAppManager{
		appMgr:  NewManager(params),
		mutex:   sync.Mutex{},
		vsMutex: make(map[serviceQueueKey]*sync.Mutex),
	}
}

func (m *mockAppManager) startNonLabelMode(namespaces []string) error {
	ls, err := labels.Parse(DefaultConfigMapLabel)
	if err != nil {
		return fmt.Errorf("failed to parse Label Selector string: %v", err)
	}
	for _, ns := range namespaces {
		err = m.appMgr.AddNamespace(ns, ls, 0)
		if nil != err {
			return fmt.Errorf(
				"Failed to add informers for namespace %v: %v", ns, err)
		}
	}
	return nil
}

func (m *mockAppManager) startLabelMode(nsLabel string) error {
	m.nsLabel = nsLabel
	nsSelector, err := labels.Parse(m.nsLabel)
	if nil != err {
		return fmt.Errorf(
			"Failed to create namespace selector for label %v", nsLabel, err)
	}
	err = m.appMgr.AddNamespaceLabelInformer(nsSelector, 0)
	if nil != err {
		return fmt.Errorf(
			"Failed to add namespace label informer with selector %v: %v",
			nsSelector, err)
	}
	return nil
}

func (m *mockAppManager) shutdown() error {
	m.appMgr.stopAppInformers()
	return nil
}

func (m *mockAppManager) resources() *Resources {
	return m.appMgr.resources
}

func (m *mockAppManager) customProfiles() map[secretKey]CustomProfile {
	return m.appMgr.customProfiles.profs
}

func (m *mockAppManager) getVsMutex(sKey serviceQueueKey) *sync.Mutex {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	mtx, ok := m.vsMutex[sKey]
	if !ok {
		mtx = &sync.Mutex{}
		m.vsMutex[sKey] = mtx
	}
	return mtx
}

func (m *mockAppManager) processNodeUpdate(obj interface{}, err error) {
	m.appMgr.ProcessNodeUpdate(obj, err)
}

func (m *mockAppManager) addConfigMap(cm *v1.ConfigMap) bool {
	ok, keys := m.appMgr.checkValidConfigMap(cm)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(cm.ObjectMeta.Namespace)
		appInf.cfgMapInformer.GetStore().Add(cm)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) updateConfigMap(cm *v1.ConfigMap) bool {
	ok, keys := m.appMgr.checkValidConfigMap(cm)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(cm.ObjectMeta.Namespace)
		appInf.cfgMapInformer.GetStore().Update(cm)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) deleteConfigMap(cm *v1.ConfigMap) bool {
	ok, keys := m.appMgr.checkValidConfigMap(cm)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(cm.ObjectMeta.Namespace)
		appInf.cfgMapInformer.GetStore().Delete(cm)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) addService(svc *v1.Service) bool {
	ok, keys := m.appMgr.checkValidService(svc)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(svc.ObjectMeta.Namespace)
		appInf.svcInformer.GetStore().Add(svc)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) updateService(svc *v1.Service) bool {
	ok, keys := m.appMgr.checkValidService(svc)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(svc.ObjectMeta.Namespace)
		appInf.svcInformer.GetStore().Update(svc)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) deleteService(svc *v1.Service) bool {
	ok, keys := m.appMgr.checkValidService(svc)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(svc.ObjectMeta.Namespace)
		appInf.svcInformer.GetStore().Delete(svc)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) addEndpoints(ep *v1.Endpoints) bool {
	ok, keys := m.appMgr.checkValidEndpoints(ep)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(ep.ObjectMeta.Namespace)
		appInf.endptInformer.GetStore().Add(ep)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) updateEndpoints(ep *v1.Endpoints) bool {
	ok, keys := m.appMgr.checkValidEndpoints(ep)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(ep.ObjectMeta.Namespace)
		appInf.endptInformer.GetStore().Update(ep)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) deleteEndpoints(ep *v1.Endpoints) bool {
	ok, keys := m.appMgr.checkValidEndpoints(ep)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(ep.ObjectMeta.Namespace)
		appInf.endptInformer.GetStore().Delete(ep)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) addIngress(ing *v1beta1.Ingress) bool {
	ok, keys := m.appMgr.checkValidIngress(ing)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(ing.ObjectMeta.Namespace)
		appInf.ingInformer.GetStore().Add(ing)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) updateIngress(ing *v1beta1.Ingress) bool {
	ok, keys := m.appMgr.checkValidIngress(ing)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(ing.ObjectMeta.Namespace)
		appInf.ingInformer.GetStore().Update(ing)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) deleteIngress(ing *v1beta1.Ingress) bool {
	ok, keys := m.appMgr.checkValidIngress(ing)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(ing.ObjectMeta.Namespace)
		appInf.ingInformer.GetStore().Delete(ing)
		for _, vsKey := range keys {
			mtx := m.getVsMutex(*vsKey)
			mtx.Lock()
			defer mtx.Unlock()
			m.appMgr.syncVirtualServer(*vsKey)
		}
	}
	return ok
}

func (m *mockAppManager) addRoute(route *routeapi.Route) bool {
	ok, vsKey := m.appMgr.checkValidRoute(route)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(route.ObjectMeta.Namespace)
		appInf.routeInformer.GetStore().Add(route)
		mtx := m.getVsMutex(*vsKey)
		mtx.Lock()
		defer mtx.Unlock()
		m.appMgr.syncVirtualServer(*vsKey)

	}
	return ok
}

func (m *mockAppManager) updateRoute(route *routeapi.Route) bool {
	ok, vsKey := m.appMgr.checkValidRoute(route)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(route.ObjectMeta.Namespace)
		appInf.routeInformer.GetStore().Update(route)
		mtx := m.getVsMutex(*vsKey)
		mtx.Lock()
		defer mtx.Unlock()
		m.appMgr.syncVirtualServer(*vsKey)
	}
	return ok
}

func (m *mockAppManager) deleteRoute(route *routeapi.Route) bool {
	ok, vsKey := m.appMgr.checkValidRoute(route)
	if ok {
		appInf, _ := m.appMgr.getNamespaceInformer(route.ObjectMeta.Namespace)
		appInf.routeInformer.GetStore().Delete(route)
		mtx := m.getVsMutex(*vsKey)
		mtx.Lock()
		defer mtx.Unlock()
		m.appMgr.syncVirtualServer(*vsKey)
	}
	return ok
}

func (m *mockAppManager) addNamespace(ns *v1.Namespace) bool {
	if "" == m.nsLabel {
		return false
	}
	_, found := ns.ObjectMeta.Labels[m.nsLabel]
	if found {
		m.appMgr.nsInformer.GetStore().Add(ns)
		m.appMgr.syncNamespace(ns.ObjectMeta.Name)
	}
	return found
}

func generateExpectedAddrs(port int32, ips []string) []string {
	var ret []string
	for _, ip := range ips {
		ret = append(ret, ip+":"+strconv.Itoa(int(port)))
	}
	return ret
}

func convertSvcPortsToEndpointPorts(svcPorts []v1.ServicePort) []v1.EndpointPort {
	eps := make([]v1.EndpointPort, len(svcPorts))
	for i, v := range svcPorts {
		eps[i].Name = v.Name
		eps[i].Port = v.Port
	}
	return eps
}

func newServicePort(name string, svcPort int32) v1.ServicePort {
	return v1.ServicePort{
		Port: svcPort,
		Name: name,
	}
}

func TestVirtualServerSendFail(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.ImmediateFail,
		Sections:  make(map[string]interface{}),
	}

	appMgr := NewManager(&Params{ConfigWriter: mw})

	require.NotPanics(t, func() {
		appMgr.outputConfig()
	})
	assert.Equal(t, 1, mw.WrittenTimes)
}

func TestVirtualServerSendFailAsync(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.AsyncFail,
		Sections:  make(map[string]interface{}),
	}

	appMgr := NewManager(&Params{ConfigWriter: mw})

	require.NotPanics(t, func() {
		appMgr.outputConfig()
	})
	assert.Equal(t, 1, mw.WrittenTimes)
}

func TestVirtualServerSendFailTimeout(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Timeout,
		Sections:  make(map[string]interface{}),
	}

	appMgr := NewManager(&Params{ConfigWriter: mw})

	require.NotPanics(t, func() {
		appMgr.outputConfig()
	})
	assert.Equal(t, 1, mw.WrittenTimes)
}

func TestGetAddresses(t *testing.T) {
	// Existing Node data
	expectedNodes := []*v1.Node{
		test.NewNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		test.NewNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		test.NewNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
		test.NewNode("node3", "3", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.3"}}),
		test.NewNode("node4", "4", false, []v1.NodeAddress{
			{"InternalIP", "127.0.0.4"}}),
		test.NewNode("node5", "5", false, []v1.NodeAddress{
			{"Hostname", "127.0.0.5"}}),
	}

	expectedReturn := []string{
		"127.0.0.1",
		"127.0.0.2",
		"127.0.0.3",
	}

	appMgr := NewManager(&Params{IsNodePort: true})

	fakeClient := fake.NewSimpleClientset()
	assert.NotNil(t, fakeClient, "Mock client cannot be nil")

	for _, expectedNode := range expectedNodes {
		node, err := fakeClient.Core().Nodes().Create(expectedNode)
		require.Nil(t, err, "Should not fail creating node")
		require.EqualValues(t, expectedNode, node, "Nodes should be equal")
	}

	appMgr.useNodeInternal = false
	nodes, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	addresses, err := appMgr.getNodeAddresses(nodes.Items)
	require.Nil(t, err, "Should not fail getting addresses")
	assert.EqualValues(t, expectedReturn, addresses,
		"Should receive the correct addresses")

	// test filtering
	expectedInternal := []string{
		"127.0.0.4",
	}

	appMgr.useNodeInternal = true
	addresses, err = appMgr.getNodeAddresses(nodes.Items)
	require.Nil(t, err, "Should not fail getting internal addresses")
	assert.EqualValues(t, expectedInternal, addresses,
		"Should receive the correct addresses")

	for _, node := range expectedNodes {
		err := fakeClient.Core().Nodes().Delete(node.ObjectMeta.Name,
			&metav1.DeleteOptions{})
		require.Nil(t, err, "Should not fail deleting node")
	}

	expectedReturn = []string{}
	appMgr.useNodeInternal = false
	nodes, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	addresses, err = appMgr.getNodeAddresses(nodes.Items)
	require.Nil(t, err, "Should not fail getting empty addresses")
	assert.EqualValues(t, expectedReturn, addresses, "Should get no addresses")
}

func validateConfig(t *testing.T, mw *test.MockWriter, expected string) {
	mw.Lock()
	_, ok := mw.Sections["resources"].(BigIPConfig)
	mw.Unlock()
	assert.True(t, ok)

	resources := struct {
		Resources BigIPConfig `json:"resources"`
	}{
		Resources: mw.Sections["resources"].(BigIPConfig),
	}

	// Read JSON from exepectedOutput into array of structs
	expectedOutput := struct {
		Resources BigIPConfig `json:"resources"`
	}{
		Resources: BigIPConfig{},
	}

	err := json.Unmarshal([]byte(expected), &expectedOutput)
	if nil != err {
		assert.Nil(t, err)
		return
	}

	for i, rs := range expectedOutput.Resources.Virtuals {
		require.Condition(t, func() bool {
			return i < len(resources.Resources.Virtuals)
		})
		assert.Contains(t, resources.Resources.Virtuals, rs)
	}
	for i, rs := range expectedOutput.Resources.Pools {
		require.Condition(t, func() bool {
			return i < len(resources.Resources.Pools)
		})
		assert.Contains(t, resources.Resources.Pools, rs)
	}
	for i, rs := range expectedOutput.Resources.Monitors {
		require.Condition(t, func() bool {
			return i < len(resources.Resources.Monitors)
		})
		assert.Contains(t, resources.Resources.Monitors, rs)
	}
}

func TestProcessNodeUpdate(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	appMgr := NewManager(&Params{
		ConfigWriter: mw,
		IsNodePort:   true,
		InitialState: true,
	})

	originalSet := []v1.Node{
		*test.NewNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		*test.NewNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		*test.NewNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
		*test.NewNode("node3", "3", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.3"}}),
		*test.NewNode("node4", "4", false, []v1.NodeAddress{
			{"InternalIP", "127.0.0.4"}}),
		*test.NewNode("node5", "5", false, []v1.NodeAddress{
			{"Hostname", "127.0.0.5"}}),
	}

	expectedOgSet := []string{
		"127.0.0.1",
		"127.0.0.2",
		"127.0.0.3",
	}

	fakeClient := fake.NewSimpleClientset(&v1.NodeList{Items: originalSet})
	assert.NotNil(t, fakeClient, "Mock client should not be nil")

	appMgr.useNodeInternal = false
	nodes, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	appMgr.ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	require.EqualValues(t, expectedOgSet, appMgr.oldNodes,
		"Should have cached correct node set")

	cachedNodes := appMgr.getNodesFromCache()
	require.EqualValues(t, appMgr.oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedOgSet, cachedNodes,
		"Cached nodes should be expected set")

	// test filtering
	expectedInternal := []string{
		"127.0.0.4",
	}

	appMgr.useNodeInternal = true
	nodes, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	appMgr.ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	require.EqualValues(t, expectedInternal, appMgr.oldNodes,
		"Should have cached correct node set")

	cachedNodes = appMgr.getNodesFromCache()
	require.EqualValues(t, appMgr.oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedInternal, cachedNodes,
		"Cached nodes should be expected set")

	// add some nodes
	_, err = fakeClient.Core().Nodes().Create(test.NewNode("nodeAdd", "nodeAdd", false,
		[]v1.NodeAddress{{"ExternalIP", "127.0.0.6"}}))
	require.Nil(t, err, "Create should not return err")

	_, err = fakeClient.Core().Nodes().Create(test.NewNode("nodeExclude", "nodeExclude",
		true, []v1.NodeAddress{{"InternalIP", "127.0.0.7"}}))

	appMgr.useNodeInternal = false
	nodes, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	appMgr.ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	expectedAddSet := append(expectedOgSet, "127.0.0.6")

	require.EqualValues(t, expectedAddSet, appMgr.oldNodes)

	cachedNodes = appMgr.getNodesFromCache()
	require.EqualValues(t, appMgr.oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedAddSet, cachedNodes,
		"Cached nodes should be expected set")

	// make no changes and re-run process
	appMgr.useNodeInternal = false
	nodes, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	appMgr.ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	expectedAddSet = append(expectedOgSet, "127.0.0.6")

	require.EqualValues(t, expectedAddSet, appMgr.oldNodes)

	cachedNodes = appMgr.getNodesFromCache()
	require.EqualValues(t, appMgr.oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedAddSet, cachedNodes,
		"Cached nodes should be expected set")

	// remove nodes
	err = fakeClient.Core().Nodes().Delete("node1", &metav1.DeleteOptions{})
	require.Nil(t, err)
	fakeClient.Core().Nodes().Delete("node2", &metav1.DeleteOptions{})
	require.Nil(t, err)
	fakeClient.Core().Nodes().Delete("node3", &metav1.DeleteOptions{})
	require.Nil(t, err)

	expectedDelSet := []string{"127.0.0.6"}

	appMgr.useNodeInternal = false
	nodes, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	appMgr.ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)

	require.EqualValues(t, expectedDelSet, appMgr.oldNodes)

	cachedNodes = appMgr.getNodesFromCache()
	require.EqualValues(t, appMgr.oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedDelSet, cachedNodes,
		"Cached nodes should be expected set")
}

func testOverwriteAddImpl(t *testing.T, isNodePort bool) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	namespace := "default"
	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   isNodePort,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	r := appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")

	resources := appMgr.resources()
	require.Equal(1, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should have entry")
	rs, ok := resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.Equal("http", rs.Virtual.Mode, "Mode should be http")

	cfgFoo = test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFooTcp})

	r = appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should have new entry")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.Equal("tcp", rs.Virtual.Mode,
		"Mode should be tcp after overwrite")
}

func TestOverwriteAddNodePort(t *testing.T) {
	testOverwriteAddImpl(t, true)
}

func TestOverwriteAddCluster(t *testing.T) {
	testOverwriteAddImpl(t, false)
}

func testServiceChangeUpdateImpl(t *testing.T, isNodePort bool) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	namespace := "default"
	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   isNodePort,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	r := appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")

	resources := appMgr.resources()
	require.Equal(1, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should have an entry")

	cfgFoo8080 := test.NewConfigMap("foomap", "2", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})

	r = appMgr.updateConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.CountOf(serviceKey{"foo", 8080, namespace}),
		"Virtual servers should have new entry")
	require.Equal(0, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should have old config removed")
}

func TestServiceChangeUpdateNodePort(t *testing.T) {
	testServiceChangeUpdateImpl(t, true)
}

func TestServiceChangeUpdateCluster(t *testing.T) {
	testServiceChangeUpdateImpl(t, false)
}

func TestServicePortsRemovedNodePort(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:      fakeClient,
		restClient:      test.CreateFakeHTTPClient(),
		ConfigWriter:    mw,
		IsNodePort:      true,
		UseNodeInternal: true,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	nodeSet := []v1.Node{
		*test.NewNode("node0", "0", false, []v1.NodeAddress{
			{"InternalIP", "127.0.0.0"}}),
		*test.NewNode("node1", "1", false, []v1.NodeAddress{
			{"InternalIP", "127.0.0.1"}}),
		*test.NewNode("node2", "2", false, []v1.NodeAddress{
			{"InternalIP", "127.0.0.2"}}),
	}

	appMgr.processNodeUpdate(nodeSet, nil)

	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := test.NewConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := test.NewConfigMap("foomap9090", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})

	foo := test.NewService("foo", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001},
			{Port: 8080, NodePort: 38001},
			{Port: 9090, NodePort: 39001}})
	r := appMgr.addService(foo)
	require.True(r, "Service should be processed")

	r = appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")

	r = appMgr.addConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")

	r = appMgr.addConfigMap(cfgFoo9090)
	require.True(r, "Config map should be processed")

	resources := appMgr.resources()
	require.Equal(3, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}))
	require.Equal(1, resources.CountOf(serviceKey{"foo", 8080, namespace}))
	require.Equal(1, resources.CountOf(serviceKey{"foo", 9090, namespace}))

	// Create a new service with less ports and update
	newFoo := test.NewService("foo", "2", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001}})

	r = appMgr.updateService(newFoo)
	require.True(r, "Service should be processed")

	require.Equal(3, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}))
	require.Equal(1, resources.CountOf(serviceKey{"foo", 8080, namespace}))
	require.Equal(1, resources.CountOf(serviceKey{"foo", 9090, namespace}))

	addrs := []string{
		"127.0.0.0",
		"127.0.0.1",
		"127.0.0.2",
	}
	rs, ok := resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.EqualValues(generateExpectedAddrs(30001, addrs),
		rs.Pools[0].PoolMemberAddrs,
		"Existing NodePort should be set on address")
	rs, ok = resources.Get(
		serviceKey{"foo", 8080, namespace}, formatConfigMapVSName(cfgFoo8080))
	require.True(ok)
	require.False(rs.MetaData.Active)
	rs, ok = resources.Get(
		serviceKey{"foo", 9090, namespace}, formatConfigMapVSName(cfgFoo9090))
	require.True(ok)
	require.False(rs.MetaData.Active)

	// Re-add port in new service
	newFoo2 := test.NewService("foo", "3", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 20001},
			{Port: 8080, NodePort: 45454}})

	r = appMgr.updateService(newFoo2)
	require.True(r, "Service should be processed")
	require.Equal(3, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}))
	require.Equal(1, resources.CountOf(serviceKey{"foo", 8080, namespace}))
	require.Equal(1, resources.CountOf(serviceKey{"foo", 9090, namespace}))

	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.EqualValues(generateExpectedAddrs(20001, addrs),
		rs.Pools[0].PoolMemberAddrs,
		"Existing NodePort should be set on address")
	rs, ok = resources.Get(
		serviceKey{"foo", 8080, namespace}, formatConfigMapVSName(cfgFoo8080))
	require.True(ok)
	require.EqualValues(generateExpectedAddrs(45454, addrs),
		rs.Pools[0].PoolMemberAddrs,
		"Existing NodePort should be set on address")
	rs, ok = resources.Get(
		serviceKey{"foo", 9090, namespace}, formatConfigMapVSName(cfgFoo9090))
	require.True(ok)
	require.False(rs.MetaData.Active)
}

func TestUpdatesConcurrentNodePort(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	assert := assert.New(t)
	require := require.New(t)

	namespace := "default"
	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgBar := test.NewConfigMap("barmap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapBar})
	foo := test.NewService("foo", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001}})
	bar := test.NewService("bar", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	nodes := []*v1.Node{
		test.NewNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		test.NewNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		test.NewNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
	}
	extraNode := test.NewNode("node3", "3", false,
		[]v1.NodeAddress{{"ExternalIP", "127.0.0.3"}})

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:      fakeClient,
		restClient:      test.CreateFakeHTTPClient(),
		ConfigWriter:    mw,
		IsNodePort:      true,
		UseNodeInternal: false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	nodeCh := make(chan struct{})
	mapCh := make(chan struct{})
	serviceCh := make(chan struct{})

	go func() {
		for _, node := range nodes {
			n, err := fakeClient.Core().Nodes().Create(node)
			require.Nil(err, "Should not fail creating node")
			require.EqualValues(node, n, "Nodes should be equal")

			nodes, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
			assert.Nil(err, "Should not fail listing nodes")
			appMgr.processNodeUpdate(nodes.Items, err)
		}

		nodeCh <- struct{}{}
	}()

	go func() {
		r := appMgr.addConfigMap(cfgFoo)
		require.True(r, "Config map should be processed")

		r = appMgr.addConfigMap(cfgBar)
		require.True(r, "Config map should be processed")

		mapCh <- struct{}{}
	}()

	go func() {
		r := appMgr.addService(foo)
		require.True(r, "Service should be processed")

		r = appMgr.addService(bar)
		require.True(r, "Service should be processed")

		serviceCh <- struct{}{}
	}()

	select {
	case <-nodeCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting node channel notification")
	}
	select {
	case <-mapCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-serviceCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting service channel notification")
	}

	validateConfig(t, mw, twoSvcsTwoNodesConfig)
	resources := appMgr.resources()

	go func() {
		err := fakeClient.Core().Nodes().Delete("node1", &metav1.DeleteOptions{})
		require.Nil(err)
		err = fakeClient.Core().Nodes().Delete("node2", &metav1.DeleteOptions{})
		require.Nil(err)
		_, err = fakeClient.Core().Nodes().Create(extraNode)
		require.Nil(err)
		nodes, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
		assert.Nil(err, "Should not fail listing nodes")
		appMgr.processNodeUpdate(nodes.Items, err)

		nodeCh <- struct{}{}
	}()

	go func() {
		r := appMgr.deleteConfigMap(cfgFoo)
		require.True(r, "Config map should be processed")
		assert.Equal(1, resources.Count())

		mapCh <- struct{}{}
	}()

	go func() {
		r := appMgr.deleteService(foo)
		require.True(r, "Service map should be processed")

		serviceCh <- struct{}{}
	}()

	select {
	case <-nodeCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting node channel notification")
	}
	select {
	case <-mapCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-serviceCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out excpecting service channel notification")
	}

	validateConfig(t, mw, oneSvcOneNodeConfig)
}

func TestProcessUpdatesNodePort(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	assert := assert.New(t)
	require := require.New(t)
	namespace := "default"

	// Create a test env with two ConfigMaps, two Services, and three Nodes
	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := test.NewConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := test.NewConfigMap("foomap9090", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})
	cfgBar := test.NewConfigMap("barmap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapBar})
	foo := test.NewService("foo", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001},
			{Port: 8080, NodePort: 38001},
			{Port: 9090, NodePort: 39001}})
	bar := test.NewService("bar", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	nodes := []v1.Node{
		*test.NewNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		*test.NewNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		*test.NewNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
	}
	extraNode := test.NewNode("node3", "3", false,
		[]v1.NodeAddress{{"ExternalIP", "127.0.0.3"}})

	addrs := []string{"127.0.0.1", "127.0.0.2"}

	fakeClient := fake.NewSimpleClientset(&v1.NodeList{Items: nodes})
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:      fakeClient,
		restClient:      test.CreateFakeHTTPClient(),
		ConfigWriter:    mw,
		IsNodePort:      true,
		UseNodeInternal: false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	n, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
	require.Nil(err)

	assert.Equal(3, len(n.Items))

	appMgr.processNodeUpdate(n.Items, err)

	// ConfigMap added
	r := appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")
	resources := appMgr.resources()
	assert.Equal(1, resources.Count())
	rs, ok := resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)

	// Second ConfigMap added
	r = appMgr.addConfigMap(cfgBar)
	require.True(r, "Config map should be processed")
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.False(rs.MetaData.Active)
	rs, ok = resources.Get(
		serviceKey{"bar", 80, namespace}, formatConfigMapVSName(cfgBar))
	require.True(ok)
	require.False(rs.MetaData.Active)

	// Service ADDED
	r = appMgr.addService(foo)
	require.True(r, "Service should be processed")
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(30001, addrs),
		rs.Pools[0].PoolMemberAddrs)

	// Second Service ADDED
	r = appMgr.addService(bar)
	require.True(r, "Service should be processed")
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"bar", 80, namespace}, formatConfigMapVSName(cfgBar))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(37001, addrs),
		rs.Pools[0].PoolMemberAddrs)

	// ConfigMap ADDED second foo port
	r = appMgr.addConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")
	assert.Equal(3, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 8080, namespace}, formatConfigMapVSName(cfgFoo8080))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(38001, addrs),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(30001, addrs),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"bar", 80, namespace}, formatConfigMapVSName(cfgBar))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(37001, addrs),
		rs.Pools[0].PoolMemberAddrs)

	// ConfigMap ADDED third foo port
	r = appMgr.addConfigMap(cfgFoo9090)
	require.True(r, "Config map should be processed")
	assert.Equal(4, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 9090, namespace}, formatConfigMapVSName(cfgFoo9090))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(39001, addrs),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"foo", 8080, namespace}, formatConfigMapVSName(cfgFoo8080))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(38001, addrs),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(30001, addrs),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"bar", 80, namespace}, formatConfigMapVSName(cfgBar))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(37001, addrs),
		rs.Pools[0].PoolMemberAddrs)

	// Nodes ADDED
	_, err = fakeClient.Core().Nodes().Create(extraNode)
	require.Nil(err)
	n, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	appMgr.processNodeUpdate(n.Items, err)
	assert.Equal(4, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(30001, append(addrs, "127.0.0.3")),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"bar", 80, namespace}, formatConfigMapVSName(cfgBar))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(37001, append(addrs, "127.0.0.3")),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"foo", 8080, namespace}, formatConfigMapVSName(cfgFoo8080))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(38001, append(addrs, "127.0.0.3")),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"foo", 9090, namespace}, formatConfigMapVSName(cfgFoo9090))
	require.True(ok)
	require.True(rs.MetaData.Active)
	assert.EqualValues(generateExpectedAddrs(39001, append(addrs, "127.0.0.3")),
		rs.Pools[0].PoolMemberAddrs)
	validateConfig(t, mw, twoSvcsFourPortsThreeNodesConfig)

	// ConfigMap DELETED third foo port
	r = appMgr.deleteConfigMap(cfgFoo9090)
	require.True(r, "Config map should be processed")
	assert.Equal(3, resources.Count())
	assert.Equal(0, resources.CountOf(serviceKey{"foo", 9090, namespace}),
		"Virtual servers should not contain removed port")
	assert.Equal(1, resources.CountOf(serviceKey{"foo", 8080, namespace}),
		"Virtual servers should contain remaining ports")
	assert.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should contain remaining ports")
	assert.Equal(1, resources.CountOf(serviceKey{"bar", 80, namespace}),
		"Virtual servers should contain remaining ports")

	// ConfigMap UPDATED second foo port
	r = appMgr.updateConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")
	assert.Equal(3, resources.Count())
	assert.Equal(1, resources.CountOf(serviceKey{"foo", 8080, namespace}),
		"Virtual servers should contain remaining ports")
	assert.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should contain remaining ports")
	assert.Equal(1, resources.CountOf(serviceKey{"bar", 80, namespace}),
		"Virtual servers should contain remaining ports")

	// ConfigMap DELETED second foo port
	r = appMgr.deleteConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")
	assert.Equal(2, resources.Count())
	assert.Equal(0, resources.CountOf(serviceKey{"foo", 8080, namespace}),
		"Virtual servers should not contain removed port")
	assert.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should contain remaining ports")
	assert.Equal(1, resources.CountOf(serviceKey{"bar", 80, namespace}),
		"Virtual servers should contain remaining ports")

	// Nodes DELETES
	err = fakeClient.Core().Nodes().Delete("node1", &metav1.DeleteOptions{})
	require.Nil(err)
	err = fakeClient.Core().Nodes().Delete("node2", &metav1.DeleteOptions{})
	require.Nil(err)
	n, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	appMgr.processNodeUpdate(n.Items, err)
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(30001, []string{"127.0.0.3"}),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"bar", 80, namespace}, formatConfigMapVSName(cfgBar))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(37001, []string{"127.0.0.3"}),
		rs.Pools[0].PoolMemberAddrs)
	validateConfig(t, mw, twoSvcsOneNodeConfig)

	// ConfigMap DELETED
	r = appMgr.deleteConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")
	assert.Equal(1, resources.Count())
	assert.Equal(0, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Config map should be removed after delete")
	validateConfig(t, mw, oneSvcOneNodeConfig)

	// Service deletedD
	r = appMgr.deleteService(bar)
	require.True(r, "Service should be processed")
	assert.Equal(1, resources.Count())
	validateConfig(t, mw, emptyConfig)
}

func TestDontCareConfigMapNodePort(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	assert := assert.New(t)
	require := require.New(t)

	namespace := "default"
	cfg := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   "bar"})
	svc := test.NewService("foo", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001}})

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   true,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	// ConfigMap ADDED
	resources := appMgr.resources()
	assert.Equal(0, resources.Count())
	// Don't wait for this config map as it will not get added to queue since
	// it is not a valid f5 ConfigMap.
	r := appMgr.addConfigMap(cfg)
	require.False(r, "Config map should not be processed")
	assert.Equal(0, resources.Count())
	r = appMgr.addService(svc)
	require.True(r, "Service should be processed")
	assert.Equal(0, resources.Count())
}

func testConfigMapKeysImpl(t *testing.T, isNodePort bool) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)
	assert := assert.New(t)

	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client should not be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   isNodePort,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	// Config map with no schema key
	noschemakey := test.NewConfigMap("noschema", "1", namespace,
		map[string]string{"data": configmapFoo})
	cfg, err := parseConfigMap(noschemakey)
	require.EqualError(err, "configmap noschema does not contain schema key",
		"Should receive no schema error")
	r := appMgr.addConfigMap(noschemakey)
	require.False(r, "Config map should not be processed")
	resources := appMgr.resources()
	require.Equal(0, resources.Count())

	// Config map with no data key
	nodatakey := test.NewConfigMap("nodata", "1", namespace, map[string]string{
		"schema": schemaUrl,
	})
	cfg, err = parseConfigMap(nodatakey)
	require.Nil(cfg, "Should not have parsed bad configmap")
	require.EqualError(err, "configmap nodata does not contain data key",
		"Should receive no data error")
	r = appMgr.addConfigMap(nodatakey)
	require.False(r, "Config map should not be processed")
	require.Equal(0, resources.Count())

	// Config map with bad json
	badjson := test.NewConfigMap("badjson", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   "///// **invalid json** /////",
	})
	cfg, err = parseConfigMap(badjson)
	require.Nil(cfg, "Should not have parsed bad configmap")
	require.EqualError(err,
		"invalid character '/' looking for beginning of value")
	r = appMgr.addConfigMap(badjson)
	require.False(r, "Config map should not be processed")
	require.Equal(0, resources.Count())

	// Config map with extra keys
	extrakeys := test.NewConfigMap("extrakeys", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo,
		"key1":   "value1",
		"key2":   "value2",
	})
	cfg, err = parseConfigMap(extrakeys)
	require.NotNil(cfg, "Config map should parse with extra keys")
	require.Nil(err, "Should not receive errors")
	r = appMgr.addConfigMap(extrakeys)
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.Count())
	resources.Delete(serviceKey{"foo", 80, namespace},
		formatConfigMapVSName(extrakeys))

	// Config map with no mode or balance
	defaultModeAndBalance := test.NewConfigMap("mode_balance", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapNoModeBalance,
	})
	cfg, err = parseConfigMap(defaultModeAndBalance)
	require.NotNil(cfg, "Config map should exist and contain default mode and balance.")
	require.Nil(err, "Should not receive errors")
	r = appMgr.addConfigMap(defaultModeAndBalance)
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.Count())

	rs, ok := resources.Get(
		serviceKey{"bar", 80, namespace}, formatConfigMapVSName(defaultModeAndBalance))
	assert.True(ok, "Config map should be accessible")
	assert.NotNil(rs, "Config map should be object")

	require.Equal("round-robin", rs.Pools[0].Balance)
	require.Equal("tcp", rs.Virtual.Mode)
	require.Equal("velcro", rs.Virtual.Partition)
	require.Equal("10.128.10.240",
		rs.Virtual.VirtualAddress.BindAddr)
	require.Equal(int32(80), rs.Virtual.VirtualAddress.Port)
}

func TestNamespaceIsolation(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)
	assert := assert.New(t)

	namespace := "default"
	wrongNamespace := "wrongnamespace"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client should not be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:      fakeClient,
		restClient:      test.CreateFakeHTTPClient(),
		ConfigWriter:    mw,
		IsNodePort:      true,
		UseNodeInternal: true,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	node := test.NewNode("node3", "3", false,
		[]v1.NodeAddress{{"InternalIP", "127.0.0.3"}})
	_, err = fakeClient.Core().Nodes().Create(node)
	require.Nil(err)
	n, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	appMgr.processNodeUpdate(n.Items, err)

	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgBar := test.NewConfigMap("foomap", "1", wrongNamespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	servFoo := test.NewService("foo", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	servBar := test.NewService("foo", "1", wrongNamespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 50000}})

	r := appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")
	resources := appMgr.resources()
	_, ok := resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	assert.True(ok, "Config map should be accessible")

	r = appMgr.addConfigMap(cfgBar)
	require.False(r, "Config map should not be processed")
	_, ok = resources.Get(
		serviceKey{"foo", 80, wrongNamespace}, formatConfigMapVSName(cfgFoo))
	assert.False(ok, "Config map should not be added if namespace does not match flag")
	assert.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should contain original config")
	assert.Equal(1, resources.Count(), "There should only be 1 virtual server")

	r = appMgr.updateConfigMap(cfgBar)
	require.False(r, "Config map should not be processed")
	_, ok = resources.Get(
		serviceKey{"foo", 80, wrongNamespace}, formatConfigMapVSName(cfgFoo))
	assert.False(ok, "Config map should not be added if namespace does not match flag")
	assert.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should contain original config")
	assert.Equal(1, resources.Count(), "There should only be 1 virtual server")

	r = appMgr.deleteConfigMap(cfgBar)
	require.False(r, "Config map should not be processed")
	_, ok = resources.Get(
		serviceKey{"foo", 80, wrongNamespace}, formatConfigMapVSName(cfgFoo))
	assert.False(ok, "Config map should not be deleted if namespace does not match flag")
	_, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	assert.True(ok, "Config map should be accessible after delete called on incorrect namespace")

	r = appMgr.addService(servFoo)
	require.True(r, "Service should be processed")
	rs, ok := resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	assert.True(ok, "Service should be accessible")
	assert.EqualValues(generateExpectedAddrs(37001, []string{"127.0.0.3"}),
		rs.Pools[0].PoolMemberAddrs,
		"Port should match initial config")

	r = appMgr.addService(servBar)
	require.False(r, "Service should not be processed")
	_, ok = resources.Get(
		serviceKey{"foo", 80, wrongNamespace}, formatConfigMapVSName(cfgFoo))
	assert.False(ok, "Service should not be added if namespace does not match flag")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	assert.True(ok, "Service should be accessible")
	assert.EqualValues(generateExpectedAddrs(37001, []string{"127.0.0.3"}),
		rs.Pools[0].PoolMemberAddrs,
		"Port should match initial config")

	r = appMgr.updateService(servBar)
	require.False(r, "Service should not be processed")
	_, ok = resources.Get(
		serviceKey{"foo", 80, wrongNamespace}, formatConfigMapVSName(cfgFoo))
	assert.False(ok, "Service should not be added if namespace does not match flag")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	assert.True(ok, "Service should be accessible")
	assert.EqualValues(generateExpectedAddrs(37001, []string{"127.0.0.3"}),
		rs.Pools[0].PoolMemberAddrs,
		"Port should match initial config")

	r = appMgr.deleteService(servBar)
	require.False(r, "Service should not be processed")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
	assert.True(ok, "Service should not have been deleted")
	assert.EqualValues(generateExpectedAddrs(37001, []string{"127.0.0.3"}),
		rs.Pools[0].PoolMemberAddrs,
		"Port should match initial config")
}

func TestConfigMapKeysNodePort(t *testing.T) {
	testConfigMapKeysImpl(t, true)
}

func TestConfigMapKeysCluster(t *testing.T) {
	testConfigMapKeysImpl(t, false)
}

func TestProcessUpdatesIAppNodePort(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	assert := assert.New(t)
	require := require.New(t)

	namespace := "default"
	// Create a test env with two ConfigMaps, two Services, and three Nodes
	cfgIapp1 := test.NewConfigMap("iapp1map", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapIApp1})
	cfgIapp2 := test.NewConfigMap("iapp2map", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapIApp2})
	iapp1 := test.NewService("iapp1", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 10101}})
	iapp2 := test.NewService("iapp2", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 20202}})
	nodes := []v1.Node{
		*test.NewNode("node0", "0", true, []v1.NodeAddress{
			{"InternalIP", "192.168.0.0"}}),
		*test.NewNode("node1", "1", false, []v1.NodeAddress{
			{"InternalIP", "192.168.0.1"}}),
		*test.NewNode("node2", "2", false, []v1.NodeAddress{
			{"InternalIP", "192.168.0.2"}}),
		*test.NewNode("node3", "3", false, []v1.NodeAddress{
			{"ExternalIP", "192.168.0.3"}}),
	}
	extraNode := test.NewNode("node4", "4", false, []v1.NodeAddress{{"InternalIP",
		"192.168.0.4"}})

	addrs := []string{"192.168.0.1", "192.168.0.2"}

	fakeClient := fake.NewSimpleClientset(&v1.NodeList{Items: nodes})
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:      fakeClient,
		restClient:      test.CreateFakeHTTPClient(),
		ConfigWriter:    mw,
		IsNodePort:      true,
		UseNodeInternal: true,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	n, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
	require.Nil(err)

	assert.Equal(4, len(n.Items))

	appMgr.processNodeUpdate(n.Items, err)

	// ConfigMap ADDED
	r := appMgr.addConfigMap(cfgIapp1)
	require.True(r, "Config map should be processed")
	resources := appMgr.resources()
	assert.Equal(1, resources.Count())
	rs, ok := resources.Get(
		serviceKey{"iapp1", 80, namespace}, formatConfigMapVSName(cfgIapp1))
	require.True(ok)

	// Second ConfigMap ADDED
	r = appMgr.addConfigMap(cfgIapp2)
	require.True(r, "Config map should be processed")
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"iapp1", 80, namespace}, formatConfigMapVSName(cfgIapp1))
	require.True(ok)

	// Service ADDED
	r = appMgr.addService(iapp1)
	require.True(r, "Service should be processed")
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"iapp1", 80, namespace}, formatConfigMapVSName(cfgIapp1))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(10101, addrs),
		rs.Pools[0].PoolMemberAddrs)

	// Second Service ADDED
	r = appMgr.addService(iapp2)
	require.True(r, "Service should be processed")
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"iapp1", 80, namespace}, formatConfigMapVSName(cfgIapp1))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(10101, addrs),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"iapp2", 80, namespace}, formatConfigMapVSName(cfgIapp2))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(20202, addrs),
		rs.Pools[0].PoolMemberAddrs)

	// ConfigMap UPDATED
	r = appMgr.updateConfigMap(cfgIapp1)
	require.True(r, "Config map should be processed")
	assert.Equal(2, resources.Count())

	// Service UPDATED
	r = appMgr.updateService(iapp1)
	require.True(r, "Service should be processed")
	assert.Equal(2, resources.Count())

	// Nodes ADDED
	_, err = fakeClient.Core().Nodes().Create(extraNode)
	require.Nil(err)
	n, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	appMgr.processNodeUpdate(n.Items, err)
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"iapp1", 80, namespace}, formatConfigMapVSName(cfgIapp1))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(10101, append(addrs, "192.168.0.4")),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"iapp2", 80, namespace}, formatConfigMapVSName(cfgIapp2))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(20202, append(addrs, "192.168.0.4")),
		rs.Pools[0].PoolMemberAddrs)
	validateConfig(t, mw, twoIappsThreeNodesConfig)

	// Nodes DELETES
	err = fakeClient.Core().Nodes().Delete("node1", &metav1.DeleteOptions{})
	require.Nil(err)
	err = fakeClient.Core().Nodes().Delete("node2", &metav1.DeleteOptions{})
	require.Nil(err)
	n, err = fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	appMgr.processNodeUpdate(n.Items, err)
	assert.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"iapp1", 80, namespace}, formatConfigMapVSName(cfgIapp1))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(10101, []string{"192.168.0.4"}),
		rs.Pools[0].PoolMemberAddrs)
	rs, ok = resources.Get(
		serviceKey{"iapp2", 80, namespace}, formatConfigMapVSName(cfgIapp2))
	require.True(ok)
	assert.EqualValues(generateExpectedAddrs(20202, []string{"192.168.0.4"}),
		rs.Pools[0].PoolMemberAddrs)
	validateConfig(t, mw, twoIappsOneNodeConfig)

	// ConfigMap DELETED
	r = appMgr.deleteConfigMap(cfgIapp1)
	require.True(r, "Config map should be processed")
	assert.Equal(1, resources.Count())
	assert.Equal(0, resources.CountOf(serviceKey{"iapp1", 80, namespace}),
		"Config map should be removed after delete")
	validateConfig(t, mw, oneIappOneNodeConfig)

	// Service DELETED
	r = appMgr.deleteService(iapp2)
	require.True(r, "Service should be processed")
	assert.Equal(1, resources.Count())
	validateConfig(t, mw, emptyConfig)
}

func testNoBindAddr(t *testing.T, isNodePort bool) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	require := require.New(t)
	assert := assert.New(t)
	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client should not be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   isNodePort,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	var configmapNoBindAddr string = string(`{
	"virtualServer": {
	    "backend": {
	      "serviceName": "foo",
	      "servicePort": 80
	    },
	    "frontend": {
	      "balance": "round-robin",
	      "mode": "http",
	      "partition": "velcro",
	      "virtualAddress": {
	        "port": 10000
	      },
	      "sslProfile": {
	        "f5ProfileName": "velcro/testcert"
	      }
	    }
	  }
	}`)
	noBindAddr := test.NewConfigMap("noBindAddr", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapNoBindAddr,
	})
	_, err = parseConfigMap(noBindAddr)
	assert.Nil(err, "Missing bindAddr should be valid")
	r := appMgr.addConfigMap(noBindAddr)
	require.True(r, "Config map should be processed")
	resources := appMgr.resources()
	require.Equal(1, resources.Count())

	rs, ok := resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(noBindAddr))
	assert.True(ok, "Config map should be accessible")
	assert.NotNil(rs, "Config map should be object")

	require.Equal("round-robin", rs.Pools[0].Balance)
	require.Equal("http", rs.Virtual.Mode)
	require.Equal("velcro", rs.Virtual.Partition)
	require.Equal("", rs.Virtual.VirtualAddress.BindAddr)
	require.Equal(int32(10000), rs.Virtual.VirtualAddress.Port)
}

func testNoVirtualAddress(t *testing.T, isNodePort bool) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	require := require.New(t)
	assert := assert.New(t)
	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client should not be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   isNodePort,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	var configmapNoVirtualAddress string = string(`{
	  "virtualServer": {
	    "backend": {
	      "serviceName": "foo",
	      "servicePort": 80
	    },
	    "frontend": {
	      "balance": "round-robin",
	      "mode": "http",
	      "partition": "velcro",
	      "sslProfile": {
	        "f5ProfileName": "velcro/testcert"
	      }
	    }
	  }
	}`)
	noVirtualAddress := test.NewConfigMap("noVirtualAddress", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapNoVirtualAddress,
	})
	_, err = parseConfigMap(noVirtualAddress)
	assert.Nil(err, "Missing virtualAddress should be valid")
	r := appMgr.addConfigMap(noVirtualAddress)
	require.True(r, "Config map should be processed")
	resources := appMgr.resources()
	require.Equal(1, resources.Count())

	rs, ok := resources.Get(
		serviceKey{"foo", 80, namespace}, formatConfigMapVSName(noVirtualAddress))
	assert.True(ok, "Config map should be accessible")
	assert.NotNil(rs, "Config map should be object")

	require.Equal("round-robin", rs.Pools[0].Balance)
	require.Equal("http", rs.Virtual.Mode)
	require.Equal("velcro", rs.Virtual.Partition)
	require.Nil(rs.Virtual.VirtualAddress)
}

func TestPoolOnly(t *testing.T) {
	testNoVirtualAddress(t, true)
	testNoBindAddr(t, true)
	testNoVirtualAddress(t, false)
	testNoBindAddr(t, false)
}

func TestSchemaValidation(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client should not be nil")

	DEFAULT_PARTITION = ""
	badjson := test.NewConfigMap("badjson", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFooInvalid,
	})
	_, err := parseConfigMap(badjson)
	assert.Contains(err.Error(),
		"virtualServer.frontend.partition: String length must be greater than or equal to 1")
	assert.Contains(err.Error(),
		"virtualServer.frontend.mode: virtualServer.frontend.mode must be one of the following: \\\"http\\\", \\\"tcp\\\"")
	assert.Contains(err.Error(),
		"virtualServer.frontend.balance: virtualServer.frontend.balance must be one of the following:")
	assert.Contains(err.Error(),
		"virtualServer.frontend.sslProfile.f5ProfileName: String length must be greater than or equal to 1")
	assert.Contains(err.Error(),
		"virtualServer.frontend.virtualAddress.bindAddr: Does not match format 'ipv4'")
	assert.Contains(err.Error(),
		"virtualServer.frontend.virtualAddress.port: Must be less than or equal to 65535")
	assert.Contains(err.Error(),
		"virtualServer.backend.serviceName: String length must be greater than or equal to 1")
	assert.Contains(err.Error(),
		"virtualServer.backend.servicePort: Must be greater than or equal to 1")
	DEFAULT_PARTITION = "velcro"
}

func TestConfigMapWrongPartition(t *testing.T) {
	require := require.New(t)
	namespace := "default"
	//Config map with wrong partition
	DEFAULT_PARTITION = "k8s" //partition the controller has been asked to watch
	wrongPartition := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	_, err := parseConfigMap(wrongPartition)
	require.NotNil(err, "Config map with wrong partition should throw an error")
	DEFAULT_PARTITION = "velcro"
}

func validateServiceIps(t *testing.T, serviceName, namespace string,
	svcPorts []v1.ServicePort, ips []string, resources *Resources) {
	for _, p := range svcPorts {
		vsMap, ok := resources.GetAll(serviceKey{serviceName, p.Port, namespace})
		require.True(t, ok)
		require.NotNil(t, vsMap)
		for _, rs := range vsMap {
			var expectedIps []string
			if ips != nil {
				expectedIps = []string{}
				for _, ip := range ips {
					ip = ip + ":" + strconv.Itoa(int(p.Port))
					expectedIps = append(expectedIps, ip)
				}
			}
			require.EqualValues(t, expectedIps, rs.Pools[0].PoolMemberAddrs,
				"nodes are not correct")
		}
	}
}

func TestVirtualServerWhenEndpointsEmpty(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	namespace := "default"
	svcName := "foo"
	emptyIps := []string{}
	readyIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}
	notReadyIps := []string{"10.2.96.3", "10.2.96.4", "10.2.96.5", "10.2.96.6"}
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
	}

	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})

	foo := test.NewService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)
	goodEndpts := test.NewEndpoints(svcName, "1", namespace, emptyIps, emptyIps,
		endptPorts)

	r := appMgr.addEndpoints(goodEndpts)
	require.True(r, "Endpoints should be processed")
	// this is for another service
	badEndpts := test.NewEndpoints("wrongSvc", "1", namespace, []string{"10.2.96.7"},
		[]string{}, endptPorts)
	r = appMgr.addEndpoints(badEndpts)
	require.True(r, "Endpoints should be processed")

	r = appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")
	r = appMgr.addService(foo)
	require.True(r, "Service should be processed")

	resources := appMgr.resources()
	require.Equal(len(svcPorts), resources.Count())
	for _, p := range svcPorts {
		require.Equal(1, resources.CountOf(serviceKey{"foo", p.Port, namespace}))
		rs, ok := resources.Get(
			serviceKey{"foo", 80, namespace}, formatConfigMapVSName(cfgFoo))
		require.True(ok)
		require.EqualValues([]string(nil), rs.Pools[0].PoolMemberAddrs)
	}

	validateServiceIps(t, svcName, namespace, svcPorts, nil, resources)

	// Move it back to ready from not ready and make sure it is re-added
	r = appMgr.updateEndpoints(test.NewEndpoints(
		svcName, "2", namespace, readyIps, notReadyIps, endptPorts))
	require.True(r, "Endpoints should be processed")
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps, resources)

	// Remove all endpoints make sure they are removed but virtual server exists
	r = appMgr.updateEndpoints(test.NewEndpoints(svcName, "3", namespace, emptyIps,
		emptyIps, endptPorts))
	require.True(r, "Endpoints should be processed")
	validateServiceIps(t, svcName, namespace, svcPorts, nil, resources)

	// Move it back to ready from not ready and make sure it is re-added
	r = appMgr.updateEndpoints(test.NewEndpoints(svcName, "4", namespace, readyIps,
		notReadyIps, endptPorts))
	require.True(r, "Endpoints should be processed")
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps, resources)
}

func TestVirtualServerWhenEndpointsChange(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	namespace := "default"
	svcName := "foo"
	emptyIps := []string{}
	readyIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}
	notReadyIps := []string{"10.2.96.3", "10.2.96.4", "10.2.96.5", "10.2.96.6"}
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
		newServicePort("port1", 8080),
		newServicePort("port2", 9090),
	}

	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := test.NewConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := test.NewConfigMap("foomap9090", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})

	foo := test.NewService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	r := appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")

	r = appMgr.addConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")

	r = appMgr.addConfigMap(cfgFoo9090)
	require.True(r, "Config map should be processed")

	r = appMgr.addService(foo)
	require.True(r, "Service should be processed")

	resources := appMgr.resources()
	require.Equal(len(svcPorts), resources.Count())
	for _, p := range svcPorts {
		require.Equal(1,
			resources.CountOf(serviceKey{"foo", p.Port, namespace}))
	}

	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)
	goodEndpts := test.NewEndpoints(svcName, "1", namespace, readyIps, notReadyIps,
		endptPorts)
	r = appMgr.addEndpoints(goodEndpts)
	require.True(r, "Endpoints should be processed")
	// this is for another service
	badEndpts := test.NewEndpoints("wrongSvc", "1", namespace, []string{"10.2.96.7"},
		[]string{}, endptPorts)
	r = appMgr.addEndpoints(badEndpts)
	require.True(r, "Endpoints should be processed")

	validateServiceIps(t, svcName, namespace, svcPorts, readyIps, resources)

	// Move an endpoint from ready to not ready and make sure it
	// goes away from virtual servers
	notReadyIps = append(notReadyIps, readyIps[len(readyIps)-1])
	readyIps = readyIps[:len(readyIps)-1]
	r = appMgr.updateEndpoints(test.NewEndpoints(svcName, "2", namespace, readyIps,
		notReadyIps, endptPorts))
	require.True(r, "Endpoints should be processed")
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps, resources)

	// Move it back to ready from not ready and make sure it is re-added
	readyIps = append(readyIps, notReadyIps[len(notReadyIps)-1])
	notReadyIps = notReadyIps[:len(notReadyIps)-1]
	r = appMgr.updateEndpoints(test.NewEndpoints(svcName, "3", namespace, readyIps,
		notReadyIps, endptPorts))
	require.True(r, "Endpoints should be processed")
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps, resources)

	// Remove all endpoints make sure they are removed but virtual server exists
	r = appMgr.updateEndpoints(test.NewEndpoints(svcName, "4", namespace, emptyIps,
		emptyIps, endptPorts))
	require.True(r, "Endpoints should be processed")
	validateServiceIps(t, svcName, namespace, svcPorts, nil, resources)

	// Move it back to ready from not ready and make sure it is re-added
	r = appMgr.updateEndpoints(test.NewEndpoints(svcName, "5", namespace, readyIps,
		notReadyIps, endptPorts))
	require.True(r, "Endpoints should be processed")
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps, resources)
}

func TestVirtualServerWhenServiceChanges(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	svcName := "foo"
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
		newServicePort("port1", 8080),
		newServicePort("port2", 9090),
	}
	svcPodIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}

	foo := test.NewService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)
	r := appMgr.addEndpoints(test.NewEndpoints(svcName, "1", namespace, svcPodIps,
		[]string{}, endptPorts))
	require.True(r, "Endpoints should be processed")

	r = appMgr.addService(foo)
	require.True(r, "Service should be processed")

	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := test.NewConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := test.NewConfigMap("foomap9090", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})

	r = appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")

	r = appMgr.addConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")

	r = appMgr.addConfigMap(cfgFoo9090)
	require.True(r, "Config map should be processed")

	resources := appMgr.resources()
	require.Equal(len(svcPorts), resources.Count())
	validateServiceIps(t, svcName, namespace, svcPorts, svcPodIps, resources)

	// delete the service and make sure the IPs go away on the VS
	r = appMgr.deleteService(foo)
	require.True(r, "Service should be processed")
	require.Equal(len(svcPorts), resources.Count())
	validateServiceIps(t, svcName, namespace, svcPorts, nil, resources)

	// re-add the service
	foo.ObjectMeta.ResourceVersion = "2"
	r = appMgr.addService(foo)
	require.True(r, "Service should be processed")
	require.Equal(len(svcPorts), resources.Count())
	validateServiceIps(t, svcName, namespace, svcPorts, svcPodIps, resources)
}

func TestVirtualServerWhenConfigMapChanges(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	svcName := "foo"
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
		newServicePort("port1", 8080),
		newServicePort("port2", 9090),
	}
	svcPodIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}

	foo := test.NewService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)

	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	r := appMgr.addService(foo)
	require.True(r, "Service should be processed")

	r = appMgr.addEndpoints(test.NewEndpoints(svcName, "1", namespace, svcPodIps,
		[]string{}, endptPorts))
	require.True(r, "Endpoints should be processed")

	// no virtual servers yet
	resources := appMgr.resources()
	require.Equal(0, resources.Count())

	// add a config map
	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	r = appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.Count())
	validateServiceIps(t, svcName, namespace, svcPorts[:1], svcPodIps, resources)

	// add another
	cfgFoo8080 := test.NewConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	r = appMgr.addConfigMap(cfgFoo8080)
	require.True(r, "Config map should be processed")
	require.Equal(2, resources.Count())
	validateServiceIps(t, svcName, namespace, svcPorts[:2], svcPodIps, resources)

	// remove first one
	r = appMgr.deleteConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.Count())
	validateServiceIps(t, svcName, namespace, svcPorts[1:2], svcPodIps, resources)
}

func TestUpdatesConcurrentCluster(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	assert := assert.New(t)
	require := require.New(t)

	namespace := "default"
	fooIps := []string{"10.2.96.1", "10.2.96.2"}
	fooPorts := []v1.ServicePort{newServicePort("port0", 8080)}
	barIps := []string{"10.2.96.0", "10.2.96.3"}
	barPorts := []v1.ServicePort{newServicePort("port1", 80)}

	cfgFoo := test.NewConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgBar := test.NewConfigMap("barmap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapBar})

	foo := test.NewService("foo", "1", namespace, v1.ServiceTypeClusterIP, fooPorts)
	bar := test.NewService("bar", "1", namespace, v1.ServiceTypeClusterIP, barPorts)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	fooEndpts := test.NewEndpoints("foo", "1", namespace, fooIps, barIps,
		convertSvcPortsToEndpointPorts(fooPorts))
	barEndpts := test.NewEndpoints("bar", "1", namespace, barIps, fooIps,
		convertSvcPortsToEndpointPorts(barPorts))
	cfgCh := make(chan struct{})
	endptCh := make(chan struct{})
	svcCh := make(chan struct{})
	resources := appMgr.resources()

	go func() {
		r := appMgr.addEndpoints(fooEndpts)
		require.True(r, "Endpoints should be processed")
		r = appMgr.addEndpoints(barEndpts)
		require.True(r, "Endpoints should be processed")

		endptCh <- struct{}{}
	}()

	go func() {
		r := appMgr.addConfigMap(cfgFoo)
		require.True(r, "Config map should be processed")

		r = appMgr.addConfigMap(cfgBar)
		require.True(r, "Config map should be processed")

		cfgCh <- struct{}{}
	}()

	go func() {
		r := appMgr.addService(foo)
		require.True(r, "Service should be processed")

		r = appMgr.addService(bar)
		require.True(r, "Service should be processed")

		svcCh <- struct{}{}
	}()

	select {
	case <-endptCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting endpoints channel notification")
	}
	select {
	case <-cfgCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-svcCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out excpecting service channel notification")
	}

	validateConfig(t, mw, twoSvcTwoPodsConfig)

	go func() {
		// delete endpoints for foo
		r := appMgr.deleteEndpoints(fooEndpts)
		require.True(r, "Endpoints should be processed")

		endptCh <- struct{}{}
	}()

	go func() {
		// delete cfgmap for foo
		r := appMgr.deleteConfigMap(cfgFoo)
		require.True(r, "Config map should be processed")

		cfgCh <- struct{}{}
	}()

	go func() {
		// Delete service for foo
		r := appMgr.deleteService(foo)
		require.True(r, "Service should be processed")

		svcCh <- struct{}{}
	}()

	select {
	case <-endptCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting endpoints channel notification")
	}
	select {
	case <-cfgCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-svcCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out excpecting service channel notification")
	}
	assert.Equal(1, resources.Count())
	validateConfig(t, mw, oneSvcTwoPodsConfig)
}

func TestNonNodePortServiceModeNodePort(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	namespace := "default"
	cfgFoo := test.NewConfigMap(
		"foomap",
		"1",
		namespace,
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		},
	)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	svcName := "foo"
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
	}

	foo := test.NewService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   true,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	r := appMgr.addService(foo)
	require.True(r, "Service should be processed")

	r = appMgr.addConfigMap(cfgFoo)
	require.True(r, "Config map should be processed")

	resources := appMgr.resources()
	require.Equal(1, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should have an entry",
	)

	foo = test.NewService(
		"foo",
		"1",
		namespace,
		"ClusterIP",
		[]v1.ServicePort{{Port: 80}},
	)

	r = appMgr.addService(foo)
	require.True(r, "Service should be processed")
	require.Equal(1, resources.Count())
	require.Equal(1, resources.CountOf(serviceKey{"foo", 80, namespace}),
		"Virtual servers should have an entry",
	)
}

func TestMultipleVirtualServersForOneBackend(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	require := require.New(t)

	namespace := "default"
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	svcPorts := []v1.ServicePort{
		newServicePort("port80", 80),
	}
	svc := test.NewService("app", "1", namespace, v1.ServiceTypeClusterIP, svcPorts)

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   false,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	r := appMgr.addService(svc)
	require.True(r, "Service should be processed")

	vsTemplate := `{
		"virtualServer": {
			"backend": {
				"serviceName": "app",
				"servicePort": 80,
				"healthMonitors": [
					{
						"interval": %d,
						"timeout": 20,
						"send": "GET /",
						"protocol": "tcp"
					}
				]
			},
			"frontend": {
				"balance": "round-robin",
				"mode": "http",
				"partition": "velcro",
				"virtualAddress": {
					"bindAddr": "10.128.10.240",
					"port": %d
				}
			}
		}
	}`

	resources := appMgr.resources()
	require.Equal(0, resources.Count())
	r = appMgr.addConfigMap(test.NewConfigMap("cmap-1", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   fmt.Sprintf(vsTemplate, 5, 80),
	}))
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.Count())
	r = appMgr.updateConfigMap(test.NewConfigMap("cmap-1", "2", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   fmt.Sprintf(vsTemplate, 5, 80),
	}))
	require.True(r, "Config map should be processed")
	require.Equal(1, resources.Count())
	r = appMgr.addConfigMap(test.NewConfigMap("cmap-2", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   fmt.Sprintf(vsTemplate, 5, 8080),
	}))
	require.True(r, "Config map should be processed")
	require.Equal(2, resources.Count())
}

func TestMultipleNamespaces(t *testing.T) {
	// Add config maps and services to 3 namespaces and ensure they only
	// are processed in the 2 namespaces we are configured to watch.
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	assert := assert.New(t)
	require := require.New(t)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   true,
	})
	ns1 := "ns1"
	ns2 := "ns2"
	nsDefault := "default"
	err := appMgr.startNonLabelMode([]string{ns1, ns2})
	require.Nil(err)
	defer appMgr.shutdown()

	node := test.NewNode("node1", "1", false,
		[]v1.NodeAddress{{"InternalIP", "127.0.0.3"}})
	_, err = fakeClient.Core().Nodes().Create(node)
	assert.Nil(err)
	n, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	appMgr.processNodeUpdate(n.Items, err)

	cfgNs1 := test.NewConfigMap("foomap", "1", ns1,
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		})
	cfgNs2 := test.NewConfigMap("foomap", "1", ns2,
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		})
	cfgNsDefault := test.NewConfigMap("foomap", "1", nsDefault,
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		})

	svcNs1 := test.NewService("foo", "1", ns1, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	svcNs2 := test.NewService("foo", "1", ns2, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 38001}})
	svcNsDefault := test.NewService("foo", "1", nsDefault, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 39001}})

	resources := appMgr.resources()
	r := appMgr.addConfigMap(cfgNs1)
	assert.True(r, "Config map should be processed")
	rs, ok := resources.Get(
		serviceKey{"foo", 80, ns1}, formatConfigMapVSName(cfgNs1))
	assert.True(ok, "Config map should be accessible")
	assert.False(rs.MetaData.Active)
	r = appMgr.addService(svcNs1)
	assert.True(r, "Service should be processed")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, ns1}, formatConfigMapVSName(cfgNs1))
	assert.True(ok, "Config map should be accessible")
	assert.True(rs.MetaData.Active)

	r = appMgr.addConfigMap(cfgNs2)
	assert.True(r, "Config map should be processed")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, ns2}, formatConfigMapVSName(cfgNs2))
	assert.True(ok, "Config map should be accessible")
	assert.False(rs.MetaData.Active)
	r = appMgr.addService(svcNs2)
	assert.True(r, "Service should be processed")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, ns2}, formatConfigMapVSName(cfgNs2))
	assert.True(ok, "Config map should be accessible")
	assert.True(rs.MetaData.Active)

	r = appMgr.addConfigMap(cfgNsDefault)
	assert.False(r, "Config map should not be processed")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, nsDefault}, formatConfigMapVSName(cfgNsDefault))
	assert.False(ok, "Config map should not be accessible")
	r = appMgr.addService(svcNsDefault)
	assert.False(r, "Service should not be processed")
	rs, ok = resources.Get(
		serviceKey{"foo", 80, nsDefault}, formatConfigMapVSName(cfgNsDefault))
	assert.False(ok, "Config map should not be accessible")
}

func TestNamespaceAddRemove(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	mock := newMockAppManager(&Params{
		KubeClient: fakeClient,
		restClient: test.CreateFakeHTTPClient(),
	})

	cfgMapSelector, err := labels.Parse(DefaultConfigMapLabel)
	require.Nil(err)

	// Add "" to watch all namespaces.
	err = mock.appMgr.AddNamespace("", cfgMapSelector, 0)
	assert.Nil(err)

	// Try to add "default" namespace, which should fail as it is covered
	// by the "" namespace.
	err = mock.appMgr.AddNamespace("default", cfgMapSelector, 0)
	assert.NotNil(err)

	// Remove "" namespace and try re-adding "default", which should work.
	err = mock.appMgr.removeNamespace("")
	assert.Nil(err)
	err = mock.appMgr.AddNamespace("default", cfgMapSelector, 0)
	assert.Nil(err)

	// Try to re-add "" namespace, which should fail.
	err = mock.appMgr.AddNamespace("", cfgMapSelector, 0)
	assert.NotNil(err)

	// Add another non-conflicting namespace, which should work.
	err = mock.appMgr.AddNamespace("myns", cfgMapSelector, 0)
	assert.Nil(err)
}

func TestNamespaceInformerAddRemove(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	mock := newMockAppManager(&Params{
		KubeClient: fakeClient,
		restClient: test.CreateFakeHTTPClient(),
	})

	cfgMapSelector, err := labels.Parse(DefaultConfigMapLabel)
	require.Nil(err)
	nsSelector, err := labels.Parse("watching")
	require.Nil(err)

	// Add a namespace to appMgr, which should prevent a namespace label
	// informer from being added.
	err = mock.appMgr.AddNamespace("default", cfgMapSelector, 0)
	assert.Nil(err)
	// Try adding a namespace label informer, which should fail
	err = mock.appMgr.AddNamespaceLabelInformer(nsSelector, 0)
	assert.NotNil(err)
	// Remove namespace added previously and retry, which should work.
	err = mock.appMgr.removeNamespace("default")
	assert.Nil(err)
	err = mock.appMgr.AddNamespaceLabelInformer(nsSelector, 0)
	assert.Nil(err)
	// Re-adding it should fail
	err = mock.appMgr.AddNamespaceLabelInformer(nsSelector, 0)
	assert.NotNil(err)
}

func TestNamespaceLabels(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}

	assert := assert.New(t)
	require := require.New(t)

	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client cannot be nil")

	appMgr := newMockAppManager(&Params{
		KubeClient:   fakeClient,
		restClient:   test.CreateFakeHTTPClient(),
		ConfigWriter: mw,
		IsNodePort:   true,
	})
	nsLabel := "watching"
	err := appMgr.startLabelMode(nsLabel)
	require.Nil(err)
	defer appMgr.shutdown()

	ns1 := test.NewNamespace("ns1", "1", map[string]string{})
	ns2 := test.NewNamespace("ns2", "1", map[string]string{"notwatching": "no"})
	ns3 := test.NewNamespace("ns3", "1", map[string]string{nsLabel: "yes"})

	node := test.NewNode("node1", "1", false,
		[]v1.NodeAddress{{"InternalIP", "127.0.0.3"}})
	_, err = fakeClient.Core().Nodes().Create(node)
	assert.Nil(err)
	n, err := fakeClient.Core().Nodes().List(metav1.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	appMgr.processNodeUpdate(n.Items, err)

	cfgNs1 := test.NewConfigMap("foomap", "1", ns1.ObjectMeta.Name,
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		})
	cfgNs2 := test.NewConfigMap("foomap", "1", ns2.ObjectMeta.Name,
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		})
	cfgNs3 := test.NewConfigMap("foomap", "1", ns3.ObjectMeta.Name,
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		})

	// Using label selectors with no matching namespaces, all adds should
	// not create any vserver entries.
	resources := appMgr.resources()
	r := appMgr.addConfigMap(cfgNs1)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs2)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs3)
	assert.False(r, "Config map should not be processed")
	_, ok := resources.Get(serviceKey{"foo", 80, ns1.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs1))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns2.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs2))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns3.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs3))
	assert.False(ok, "Config map should not be accessible")

	// Add a namespace with no label, should still not create any resources.
	r = appMgr.addNamespace(ns1)
	assert.False(r)
	r = appMgr.addConfigMap(cfgNs1)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs2)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs3)
	assert.False(r, "Config map should not be processed")
	_, ok = resources.Get(serviceKey{"foo", 80, ns1.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs1))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns2.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs2))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns3.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs3))
	assert.False(ok, "Config map should not be accessible")

	// Add a namespace with a mismatched label, should still not create any
	// resources.
	r = appMgr.addNamespace(ns2)
	assert.False(r)
	r = appMgr.addConfigMap(cfgNs1)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs2)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs3)
	assert.False(r, "Config map should not be processed")
	_, ok = resources.Get(serviceKey{"foo", 80, ns1.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs1))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns2.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs2))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns3.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs3))
	assert.False(ok, "Config map should not be accessible")

	// Add a namespace with a matching label and make sure the config map that
	// references that namespace is added to resources.
	r = appMgr.addNamespace(ns3)
	assert.True(r)
	r = appMgr.addConfigMap(cfgNs1)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs2)
	assert.False(r, "Config map should not be processed")
	r = appMgr.addConfigMap(cfgNs3)
	assert.True(r, "Config map should be processed")
	_, ok = resources.Get(serviceKey{"foo", 80, ns1.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs1))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns2.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs2))
	assert.False(ok, "Config map should not be accessible")
	rs, ok := resources.Get(serviceKey{"foo", 80, ns3.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs3))
	assert.True(ok, "Config map should be accessible")
	assert.False(rs.MetaData.Active)

	// Add services corresponding to the config maps. The only change expected
	// is the service in ns3 should become active.
	svcNs1 := test.NewService("foo", "1", ns1.ObjectMeta.Name, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	svcNs2 := test.NewService("foo", "1", ns2.ObjectMeta.Name, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 38001}})
	svcNs3 := test.NewService("foo", "1", ns3.ObjectMeta.Name, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 39001}})
	r = appMgr.addService(svcNs1)
	assert.False(r, "Service should not be processed")
	r = appMgr.addService(svcNs2)
	assert.False(r, "Service should not be processed")
	r = appMgr.addService(svcNs3)
	assert.True(r, "Service should be processed")
	_, ok = resources.Get(serviceKey{"foo", 80, ns1.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs1))
	assert.False(ok, "Config map should not be accessible")
	_, ok = resources.Get(serviceKey{"foo", 80, ns2.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs2))
	assert.False(ok, "Config map should not be accessible")
	rs, ok = resources.Get(serviceKey{"foo", 80, ns3.ObjectMeta.Name},
		formatConfigMapVSName(cfgNs3))
	assert.True(ok, "Config map should be accessible")
	assert.True(rs.MetaData.Active)
}

func TestVirtualServerForIngress(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	require := require.New(t)
	assert := assert.New(t)
	fakeClient := fake.NewSimpleClientset()
	fakeRecorder := record.NewFakeRecorder(100)
	require.NotNil(fakeClient, "Mock client should not be nil")
	require.NotNil(fakeRecorder, "Mock recorder should not be nil")
	namespace := "default"

	appMgr := newMockAppManager(&Params{
		KubeClient:    fakeClient,
		ConfigWriter:  mw,
		restClient:    test.CreateFakeHTTPClient(),
		IsNodePort:    true,
		EventRecorder: fakeRecorder,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	ingressConfig := v1beta1.IngressSpec{
		Backend: &v1beta1.IngressBackend{
			ServiceName: "foo",
			ServicePort: intstr.IntOrString{IntVal: 80},
		},
	}
	// Add a new Ingress
	ingress := test.NewIngress("ingress", "1", namespace, ingressConfig,
		map[string]string{
			"virtual-server.f5.com/ip":        "1.2.3.4",
			"virtual-server.f5.com/partition": "velcro",
		})
	r := appMgr.addIngress(ingress)
	require.True(r, "Ingress resource should be processed")
	resources := appMgr.resources()
	require.Equal(1, resources.Count())
	// Associate a service
	fooSvc := test.NewService("foo", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	r = appMgr.addService(fooSvc)
	assert.True(r, "Service should be processed")

	rs, ok := resources.Get(
		serviceKey{"foo", 80, "default"}, "default_ingress-ingress_http")
	assert.True(ok, "Ingress should be accessible")
	assert.NotNil(rs, "Ingress should be object")
	assert.True(rs.MetaData.Active)

	require.Equal("round-robin", rs.Pools[0].Balance)
	require.Equal("http", rs.Virtual.Mode)
	require.Equal("velcro", rs.Virtual.Partition)
	require.Equal("1.2.3.4", rs.Virtual.VirtualAddress.BindAddr)
	require.Equal(int32(80), rs.Virtual.VirtualAddress.Port)
	// Update the Ingress resource
	ingress2 := test.NewIngress("ingress", "1", namespace, ingressConfig,
		map[string]string{
			"virtual-server.f5.com/ip":        "5.6.7.8",
			"virtual-server.f5.com/partition": "velcro2",
			"virtual-server.f5.com/http-port": "443",
		})
	r = appMgr.updateIngress(ingress2)
	require.True(r, "Ingress resource should be processed")
	require.Equal(1, resources.Count())

	rs, ok = resources.Get(
		serviceKey{"foo", 80, "default"}, "default_ingress-ingress_http")
	assert.True(ok, "Ingress should be accessible")
	assert.NotNil(rs, "Ingress should be object")

	require.Equal("velcro2", rs.Virtual.Partition)
	require.Equal("5.6.7.8", rs.Virtual.VirtualAddress.BindAddr)
	require.Equal(int32(443), rs.Virtual.VirtualAddress.Port)
	// Delete the Ingress resource
	r = appMgr.deleteIngress(ingress2)
	require.True(r, "Ingress resource should be processed")
	require.Equal(0, resources.Count())

	// Multi-service Ingress
	ingressConfig = v1beta1.IngressSpec{
		Rules: []v1beta1.IngressRule{
			{Host: "host1",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{
							{Path: "/foo",
								Backend: v1beta1.IngressBackend{
									ServiceName: "foo",
									ServicePort: intstr.IntOrString{IntVal: 80},
								},
							},
							{Path: "/bar",
								Backend: v1beta1.IngressBackend{
									ServiceName: "bar",
									ServicePort: intstr.IntOrString{IntVal: 80},
								},
							},
						},
					},
				},
			},
			{Host: "host2",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{
							{Path: "/foo",
								Backend: v1beta1.IngressBackend{
									ServiceName: "foo",
									ServicePort: intstr.IntOrString{IntVal: 80},
								},
							},
							{Path: "/foobar",
								Backend: v1beta1.IngressBackend{
									ServiceName: "foobar",
									ServicePort: intstr.IntOrString{IntVal: 80},
								},
							},
						},
					},
				},
			},
		},
	}
	barSvc := test.NewService("bar", "2", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37002}})
	r = appMgr.addService(barSvc)
	assert.True(r, "Service should be processed")
	foobarSvc := test.NewService("foobar", "3", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37003}})
	r = appMgr.addService(foobarSvc)
	assert.True(r, "Service should be processed")

	ingress3 := test.NewIngress("ingress", "2", namespace, ingressConfig,
		map[string]string{
			"virtual-server.f5.com/ip":        "1.2.3.4",
			"virtual-server.f5.com/partition": "velcro",
		})
	r = appMgr.addIngress(ingress3)
	require.True(r, "Ingress resource should be processed")
	// 4 rules, but only 3 backends specified. We should have 3 keys stored, one for
	// each backend
	require.Equal(3, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 80, "default"}, "default_ingress-ingress_http")
	require.Equal(4, len(rs.Policies[0].Rules))
	appMgr.deleteService(fooSvc)
	require.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"bar", 80, "default"}, "default_ingress-ingress_http")
	require.Equal(2, len(rs.Policies[0].Rules))

	appMgr.deleteIngress(ingress3)
	appMgr.addService(fooSvc)
	ingressConfig = v1beta1.IngressSpec{
		Rules: []v1beta1.IngressRule{
			{Host: "",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{
							{Path: "/foo",
								Backend: v1beta1.IngressBackend{
									ServiceName: "foo",
									ServicePort: intstr.IntOrString{IntVal: 80},
								},
							},
							{Path: "/bar",
								Backend: v1beta1.IngressBackend{
									ServiceName: "bar",
									ServicePort: intstr.IntOrString{IntVal: 80},
								},
							},
						},
					},
				},
			},
		},
	}
	ingress4 := test.NewIngress("ingress", "3", namespace, ingressConfig,
		map[string]string{
			"virtual-server.f5.com/ip":        "1.2.3.4",
			"virtual-server.f5.com/partition": "velcro",
		})
	r = appMgr.addIngress(ingress4)
	require.True(r, "Ingress resource should be processed")
	require.Equal(2, resources.Count())
	rs, ok = resources.Get(
		serviceKey{"foo", 80, "default"}, "default_ingress-ingress_http")
	require.Equal(2, len(rs.Policies[0].Rules))
}

func TestIngressSslProfile(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	require := require.New(t)
	assert := assert.New(t)
	fakeClient := fake.NewSimpleClientset()
	fakeRecorder := record.NewFakeRecorder(100)
	require.NotNil(fakeClient, "Mock client should not be nil")
	require.NotNil(fakeRecorder, "Mock recorder should not be nil")
	namespace := "default"
	svcName := "foo"
	var svcPort int32 = 443
	svcKey := serviceKey{
		Namespace:   namespace,
		ServiceName: svcName,
		ServicePort: svcPort,
	}
	sslProfileName1 := "velcro/theSslProfileName"
	sslProfileName2 := "common/anotherSslProfileName"

	appMgr := newMockAppManager(&Params{
		KubeClient:    fakeClient,
		ConfigWriter:  mw,
		restClient:    test.CreateFakeHTTPClient(),
		IsNodePort:    false,
		EventRecorder: fakeRecorder,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	spec := v1beta1.IngressSpec{
		TLS: []v1beta1.IngressTLS{
			{
				SecretName: sslProfileName1,
			},
			{
				SecretName: sslProfileName2,
			},
		},
		Backend: &v1beta1.IngressBackend{
			ServiceName: svcName,
			ServicePort: intstr.IntOrString{IntVal: svcPort},
		},
	}
	fooIng := test.NewIngress("ingress", "1", namespace, spec,
		map[string]string{
			"virtual-server.f5.com/ip":        "1.2.3.4",
			"virtual-server.f5.com/partition": "velcro",
		})
	svcPorts := []v1.ServicePort{newServicePort("port0", svcPort)}
	fooSvc := test.NewService(svcName, "1", namespace, v1.ServiceTypeClusterIP,
		svcPorts)
	emptyIps := []string{}
	readyIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}
	endpts := test.NewEndpoints(svcName, "1", namespace, readyIps, emptyIps,
		convertSvcPortsToEndpointPorts(svcPorts))

	// Add ingress, service, and endpoints objects and make sure the
	// ssl-profile set in the ingress object shows up in the virtual server.
	r := appMgr.addIngress(fooIng)
	assert.True(r, "Ingress resource should be processed")
	r = appMgr.addService(fooSvc)
	assert.True(r, "Service should be processed")
	r = appMgr.addEndpoints(endpts)
	assert.True(r, "Endpoints should be processed")
	resources := appMgr.resources()
	assert.Equal(2, resources.Count())
	assert.Equal(2, resources.CountOf(svcKey))
	httpCfg, found := resources.Get(svcKey, formatIngressVSName(fooIng, "http"))
	assert.True(found)
	require.NotNil(httpCfg)

	httpsCfg, found := resources.Get(svcKey, formatIngressVSName(fooIng, "https"))
	assert.True(found)
	require.NotNil(httpsCfg)
	secretArray := []string{
		formatIngressSslProfileName(sslProfileName1),
		formatIngressSslProfileName(sslProfileName2),
	}
	sort.Strings(secretArray)
	assert.Equal(secretArray, httpsCfg.Virtual.GetFrontendSslProfileNames())

	// No annotations were specified to control http redirect, check that
	// we are in the default state 2.
	require.Equal(1, len(httpCfg.Policies))
	require.Equal(1, len(httpCfg.Policies[0].Rules))
	assert.Equal(httpRedirectRuleName, httpCfg.Policies[0].Rules[0].Name)

	// Set the annotations the same as default and recheck
	fooIng.ObjectMeta.Annotations[ingressSslRedirect] = "true"
	fooIng.ObjectMeta.Annotations[ingressAllowHttp] = "false"
	r = appMgr.addIngress(fooIng)
	httpCfg, found = resources.Get(svcKey, formatIngressVSName(fooIng, "http"))
	assert.True(found)
	require.NotNil(httpCfg)
	assert.True(r, "Ingress resource should be processed")
	require.Equal(1, len(httpCfg.Policies))
	require.Equal(1, len(httpCfg.Policies[0].Rules))
	assert.Equal(httpRedirectRuleName, httpCfg.Policies[0].Rules[0].Name)

	// Now test state 1.
	fooIng.ObjectMeta.Annotations[ingressSslRedirect] = "false"
	fooIng.ObjectMeta.Annotations[ingressAllowHttp] = "false"
	r = appMgr.addIngress(fooIng)
	httpsCfg, found = resources.Get(svcKey, formatIngressVSName(fooIng, "https"))
	assert.True(found)
	require.NotNil(httpsCfg)
	assert.True(r, "Ingress resource should be processed")
	assert.Equal(1, resources.Count())
	assert.Equal(1, resources.CountOf(svcKey))
	require.Equal(0, len(httpsCfg.Policies))
	assert.Equal(secretArray, httpsCfg.Virtual.GetFrontendSslProfileNames())

	// Now test state 3.
	fooIng.ObjectMeta.Annotations[ingressSslRedirect] = "false"
	fooIng.ObjectMeta.Annotations[ingressAllowHttp] = "true"
	r = appMgr.addIngress(fooIng)
	httpCfg, found = resources.Get(svcKey, formatIngressVSName(fooIng, "http"))
	assert.True(found)
	require.NotNil(httpCfg)
	assert.True(r, "Ingress resource should be processed")
	require.Equal(0, len(httpCfg.Policies))

	// Clear out TLS in the ingress, but use default http redirect settings.
	fooIng.Spec.TLS = nil
	delete(fooIng.ObjectMeta.Annotations, ingressSslRedirect)
	delete(fooIng.ObjectMeta.Annotations, ingressAllowHttp)
	r = appMgr.addIngress(fooIng)
	assert.True(r, "Ingress resource should be processed")
	assert.Equal(1, resources.Count())
	assert.Equal(1, resources.CountOf(svcKey))
	httpCfg, found = resources.Get(svcKey, formatIngressVSName(fooIng, "http"))
	assert.True(found)
	require.NotNil(httpCfg)
	require.Equal(0, len(httpCfg.Policies))

	httpsCfg, found = resources.Get(svcKey, formatIngressVSName(fooIng, "https"))
	assert.False(found)
	require.Nil(httpsCfg)
}

func TestSecretSslProfile(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	require := require.New(t)
	assert := assert.New(t)
	fakeClient := fake.NewSimpleClientset()
	fakeRecorder := record.NewFakeRecorder(100)
	require.NotNil(fakeClient, "Mock client should not be nil")
	require.NotNil(fakeRecorder, "Mock recorder should not be nil")
	namespace := "default"

	appMgr := newMockAppManager(&Params{
		KubeClient:    fakeClient,
		ConfigWriter:  mw,
		restClient:    test.CreateFakeHTTPClient(),
		IsNodePort:    false,
		EventRecorder: fakeRecorder,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	// Create a secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("testcert"),
			"tls.key": []byte("testkey"),
		},
	}
	_, err = fakeClient.Core().Secrets(namespace).Create(secret)
	require.Nil(err)

	spec := v1beta1.IngressSpec{
		TLS: []v1beta1.IngressTLS{
			{
				SecretName: secret.ObjectMeta.Name,
			},
		},
		Backend: &v1beta1.IngressBackend{
			ServiceName: "foo",
			ServicePort: intstr.IntOrString{IntVal: 80},
		},
	}
	// Test for Ingress
	ingress := test.NewIngress("ingress", "1", namespace, spec,
		map[string]string{
			"virtual-server.f5.com/ip":        "1.2.3.4",
			"virtual-server.f5.com/partition": "velcro",
		})
	appMgr.addIngress(ingress)

	customProfiles := appMgr.customProfiles()
	assert.Equal(1, len(customProfiles))
	// Test for ConfigMap
	var configmapSecret string = string(`{
	"virtualServer": {
	    "backend": {
	      "serviceName": "foo",
	      "servicePort": 80
	    },
	    "frontend": {
	      "partition": "velcro",
	      "virtualAddress": {
	        "port": 10000
	      },
	      "sslProfile": {
	        "f5ProfileName": "secret"
	      }
	    }
	  }
	}`)
	secretCfg := test.NewConfigMap("secretCfg", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapSecret,
	})
	appMgr.addConfigMap(secretCfg)
	assert.Equal(2, len(customProfiles))
	appMgr.deleteConfigMap(secretCfg)
	assert.Equal(1, len(customProfiles))
}

func TestVirtualServerForRoute(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	require := require.New(t)
	assert := assert.New(t)
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client should not be nil")
	namespace := "default"

	appMgr := newMockAppManager(&Params{
		KubeClient:    fakeClient,
		ConfigWriter:  mw,
		restClient:    test.CreateFakeHTTPClient(),
		RouteClientV1: test.CreateFakeHTTPClient(),
		IsNodePort:    true,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	spec := routeapi.RouteSpec{
		Host: "foobar.com",
		Path: "/foo",
		To: routeapi.RouteTargetReference{
			Kind: "Service",
			Name: "foo",
		},
		TLS: &routeapi.TLSConfig{
			Termination: "edge",
			Certificate: "cert",
			Key:         "key",
		},
	}
	route := test.NewRoute("route", "1", namespace, spec)
	r := appMgr.addRoute(route)
	require.True(r, "Route resource should be processed")

	resources := appMgr.resources()
	// Associate a service
	fooSvc := test.NewService("foo", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 443, NodePort: 37001}})
	r = appMgr.addService(fooSvc)
	assert.True(r, "Service should be processed")
	require.Equal(2, resources.Count())

	rs, ok := resources.Get(
		serviceKey{"foo", 443, "default"}, "openshift_default_https")
	require.True(ok, "Route should be accessible")
	require.NotNil(rs, "Route should be object")
	assert.True(rs.MetaData.Active)
	assert.Equal(1, len(rs.Policies[0].Rules))

	customProfiles := appMgr.customProfiles()
	assert.Equal(1, len(customProfiles))

	spec = routeapi.RouteSpec{
		Host: "barfoo.com",
		Path: "/bar",
		To: routeapi.RouteTargetReference{
			Kind: "Service",
			Name: "bar",
		},
		TLS: &routeapi.TLSConfig{
			Termination: "edge",
			Certificate: "cert",
			Key:         "key",
		},
	}
	route2 := test.NewRoute("route2", "1", namespace, spec)
	r = appMgr.addRoute(route2)
	require.True(r, "Route resource should be processed")
	resources = appMgr.resources()
	// Associate a service
	barSvc := test.NewService("bar", "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	appMgr.addService(barSvc)
	r = assert.True(r, "Service should be processed")
	require.Equal(4, resources.Count())

	rs, ok = resources.Get(
		serviceKey{"bar", 80, "default"}, "openshift_default_https")
	require.True(ok, "Route should be accessible")
	require.NotNil(rs, "Route should be object")
	assert.True(rs.MetaData.Active)
	assert.Equal(2, len(rs.Policies[0].Rules))

	customProfiles = appMgr.customProfiles()
	assert.Equal(2, len(customProfiles))

	// Delete a Route resource
	r = appMgr.deleteRoute(route2)
	require.True(r, "Route resource should be processed")
	require.Equal(2, resources.Count())
	assert.Equal(1, len(rs.Policies[0].Rules))
	assert.Equal(1, len(customProfiles))
}

func TestPassthroughRoute(t *testing.T) {
	mw := &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	require := require.New(t)
	assert := assert.New(t)
	fakeClient := fake.NewSimpleClientset()
	require.NotNil(fakeClient, "Mock client should not be nil")
	namespace := "default"

	appMgr := newMockAppManager(&Params{
		KubeClient:    fakeClient,
		ConfigWriter:  mw,
		restClient:    test.CreateFakeHTTPClient(),
		RouteClientV1: test.CreateFakeHTTPClient(),
		IsNodePort:    true,
	})
	err := appMgr.startNonLabelMode([]string{namespace})
	require.Nil(err)
	defer appMgr.shutdown()

	// create 2 services and routes
	hostName1 := "foobar.com"
	svcName1 := "foo"
	spec := routeapi.RouteSpec{
		Host: hostName1,
		Path: "/foo",
		To: routeapi.RouteTargetReference{
			Kind: "Service",
			Name: svcName1,
		},
		TLS: &routeapi.TLSConfig{
			Termination: routeapi.TLSTerminationPassthrough,
		},
	}
	route1 := test.NewRoute("rt1", "1", namespace, spec)
	r := appMgr.addRoute(route1)
	assert.True(r, "Route resource should be processed")

	resources := appMgr.resources()
	fooSvc := test.NewService(svcName1, "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 443, NodePort: 37001}})
	r = appMgr.addService(fooSvc)
	assert.True(r, "Service should be processed")
	assert.Equal(2, resources.Count())

	hostName2 := "barfoo.com"
	svcName2 := "bar"
	spec = routeapi.RouteSpec{
		Host: hostName2,
		Path: "/bar",
		To: routeapi.RouteTargetReference{
			Kind: "Service",
			Name: svcName2,
		},
		TLS: &routeapi.TLSConfig{
			Termination:                   routeapi.TLSTerminationPassthrough,
			InsecureEdgeTerminationPolicy: routeapi.InsecureEdgeTerminationPolicyRedirect,
		},
	}
	route2 := test.NewRoute("rt2", "1", namespace, spec)
	r = appMgr.addRoute(route2)
	require.True(r, "Route resource should be processed")
	resources = appMgr.resources()
	barSvc := test.NewService(svcName2, "1", namespace, "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	appMgr.addService(barSvc)
	r = assert.True(r, "Service should be processed")
	assert.Equal(4, resources.Count())

	// Check state.
	rs, ok := resources.Get(
		serviceKey{svcName1, 443, namespace}, "openshift_default_https")
	assert.True(ok, "Route should be accessible")
	require.NotNil(rs, "Route should be object")
	assert.True(rs.MetaData.Active)
	assert.Equal(0, len(rs.Policies))
	assert.Equal(1, len(rs.Virtual.IRules))
	expectedIRuleName := fmt.Sprintf("/%s/%s",
		DEFAULT_PARTITION, sslPassthroughIRuleName)
	assert.Equal(expectedIRuleName, rs.Virtual.IRules[0])
	hostDgKey := nameRef{
		Name:      passthroughHostsDgName,
		Partition: DEFAULT_PARTITION,
	}
	hostDg, found := appMgr.appMgr.intDgMap[hostDgKey]
	assert.True(found)
	assert.Equal(2, len(hostDg.Records))
	assert.Equal(hostName1, hostDg.Records[1].Name)
	assert.Equal(hostName2, hostDg.Records[0].Name)
	assert.Equal(formatRoutePoolName(route1), hostDg.Records[1].Data)
	assert.Equal(formatRoutePoolName(route2), hostDg.Records[0].Data)

	rs, ok = resources.Get(
		serviceKey{svcName2, 80, namespace}, "openshift_default_http")
	require.True(ok, "Route should be accessible")
	require.NotNil(rs, "Route should be object")
	assert.True(rs.MetaData.Active)
	assert.Equal(1, len(rs.Policies))
	assert.Equal(1, len(rs.Policies[0].Rules))
	assert.Equal(httpRedirectRuleName, rs.Policies[0].Rules[0].Name)

	// Delete a Route resource and make sure the data groups are cleaned up.
	r = appMgr.deleteRoute(route2)
	require.True(r, "Route resource should be processed")
	require.Equal(2, resources.Count())
	hostDg, found = appMgr.appMgr.intDgMap[hostDgKey]
	assert.True(found)
	assert.Equal(1, len(hostDg.Records))
	assert.Equal(hostName1, hostDg.Records[0].Name)
	assert.Equal(formatRoutePoolName(route1), hostDg.Records[0].Data)
}
