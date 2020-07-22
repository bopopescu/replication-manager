// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import "github.com/signal18/replication-manager/cluster"

func testFailoverNoRplChecksNoSemiSyncMainHeartbeat(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {
	cluster.SetRplMaxDelay(0)
	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)

		return false
	}
	SaveMainURL := cluster.GetMain().URL

	cluster.LogPrintf(LvlInfo, "Main is %s", cluster.GetMain().URL)
	cluster.SetInteractive(false)
	cluster.SetFailLimit(5)
	cluster.SetFailTime(0)
	cluster.SetFailoverCtr(0)
	cluster.SetCheckFalsePositiveHeartbeat(true)
	cluster.SetRplChecks(false)
	cluster.SetRplMaxDelay(20)
	cluster.CheckFailed()
	cluster.FailoverNow()
	cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)
	if cluster.GetMain().URL != SaveMainURL {
		cluster.LogPrintf("TEST", "Old main %s ==  Next main %s  ", SaveMainURL, cluster.GetMain().URL)
		return false
	}
	err = cluster.EnableSemisync()
	if err != nil {
		cluster.LogPrintf("ERROR:", "%s", err)

		return false
	}
	return true
}
