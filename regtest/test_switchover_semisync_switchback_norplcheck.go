// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import (
	"time"

	"github.com/signal18/replication-manager/cluster"
	"github.com/signal18/replication-manager/utils/dbhelper"
)

func testSwitchover2TimesReplicationOkSemiSyncNoRplCheck(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetRplChecks(false)
	cluster.SetRplMaxDelay(0)
	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)
		return false
	}
	time.Sleep(2 * time.Second)

	for i := 0; i < 2; i++ {
		result, err := dbhelper.WriteConcurrent2(cluster.GetMain().DSN, 10)
		if err != nil {
			cluster.LogPrintf(LvlErr, "%s %s", err.Error(), result)
		}
		cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)
		SaveMainURL := cluster.GetMain().URL
		cluster.SwitchoverWaitTest()
		cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)
		if SaveMainURL == cluster.GetMain().URL {
			cluster.LogPrintf(LvlErr, "same server URL after switchover")
			return false
		}
	}
	time.Sleep(2 * time.Second)
	for _, s := range cluster.GetSubordinates() {
		if s.IsReplicationBroken() {
			cluster.LogPrintf(LvlErr, "Subordinate  %s issue on replication", s.URL)
			return false
		}
		if s.GetReplicationServerID() != cluster.GetMain().ServerID {
			cluster.LogPrintf(LvlErr, "Replication is  pointing to wrong main %s ", cluster.GetMain().ServerID)
			return false
		}
	}
	return true
}
