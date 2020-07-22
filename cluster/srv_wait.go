// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.
// Redistribution/Reuse of this code is permitted under the GNU v3 license, as
// an additional term, ALL code must carry the original Author(s) credit in comment form.
// See LICENSE in this directory for the integral text.

package cluster

import "github.com/signal18/replication-manager/utils/dbhelper"

func (server *ServerMonitor) WaitSyncToMain(main *ServerMonitor) {
	server.ClusterGroup.LogPrintf(LvlInfo, "Waiting for subordinate %s to sync", server.URL)
	if server.DBVersion.Flavor == "MariaDB" {
		logs, err := dbhelper.MainWaitGTID(server.Conn, main.GTIDBinlogPos.Sprint(), 30)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "MainFailover", LvlErr, "Failed MainWaitGTID, %s", err)

	} else {
		logs, err := dbhelper.MainPosWait(server.Conn, main.BinaryLogFile, main.BinaryLogPos, 30)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "MainFailover", LvlErr, "Failed MainPosWait, %s", err)
	}

	if server.ClusterGroup.Conf.LogLevel > 2 {
		server.LogReplPostion()
	}
}
