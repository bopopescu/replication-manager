// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.
// Redistribution/Reuse of this code is permitted under the GNU v3 license, as
// an additional term, ALL code must carry the original Author(s) credit in comment form.
// See LICENSE in this directory for the integral text.

package cluster

import (
	"strconv"
	"strings"

	"github.com/micro/go-micro/registry"
)

func (cluster *Cluster) initConsul() error {
	var opt registry.Options
	//opt := consul.DefaultConfig()
	if cluster.Conf.RegistryConsul == false || cluster.IsActive() == false {
		return nil
	}
	opt.Addrs = strings.Split(cluster.Conf.RegistryHosts, ",")
	//DefaultRegistry()
	//opt := registry.DefaultRegistry
	reg := registry.NewRegistry()
	if cluster.GetMain() != nil {

		port, _ := strconv.Atoi(cluster.GetMain().Port)
		writesrv := map[string][]*registry.Service{
			"write": []*registry.Service{
				{
					Name:    "write_" + cluster.GetName(),
					Version: "0.0.0",
					Nodes: []*registry.Node{
						{
							Id:      "write_" + cluster.GetName(),
							Address: cluster.GetMain().Host,
							Port:    port,
						},
					},
				},
			},
		}

		cluster.LogPrintf(LvlInfo, "Register consul main ID %s with host %s", "write_"+cluster.GetName(), cluster.GetMain().URL)
		delservice, err := reg.GetService("write_" + cluster.GetName())
		if err != nil {
			for _, service := range delservice {

				if err := reg.Deregister(service); err != nil {
					cluster.LogPrintf(LvlErr, "Unexpected deregister error: %v", err)
				}
			}
		}
		//reg := registry.NewRegistry()
		for _, v := range writesrv {
			for _, service := range v {

				if err := reg.Register(service); err != nil {
					cluster.LogPrintf(LvlErr, "Unexpected register error: %v", err)
				}

			}
		}

	}

	for _, srv := range cluster.Servers {
		var readsrv registry.Service
		readsrv.Name = "read_" + cluster.GetName()
		readsrv.Version = "0.0.0"
		var readnodes []*registry.Node
		var node registry.Node
		node.Id = srv.Id
		node.Address = srv.Host
		port, _ := strconv.Atoi(srv.Port)
		node.Port = port
		readnodes = append(readnodes, &node)
		readsrv.Nodes = readnodes

		if err := reg.Deregister(&readsrv); err != nil {
			cluster.LogPrintf(LvlErr, "Unexpected consul deregister error for server %s: %v", srv.URL, err)
		}
		if srv.State != stateFailed && srv.State != stateMaintenance && srv.State != stateUnconn {
			if (srv.IsSubordinate && srv.HasReplicationIssue() == false) || (srv.IsMain() && cluster.Conf.PRXServersReadOnMain) {
				cluster.LogPrintf(LvlInfo, "Register consul read service  %s %s", srv.Id, srv.URL)
				if err := reg.Register(&readsrv); err != nil {
					cluster.LogPrintf(LvlErr, "Unexpected consul register error for server %s: %v", srv.URL, err)
				}
			}
		}
	}

	return nil
}
