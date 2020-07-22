// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import "github.com/signal18/replication-manager/cluster"

func testFailoverNumberFailureLimitReach(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {
	cluster.SetRplMaxDelay(0)
	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)

		return false
	}

	SaveMain := cluster.GetMain()
	SaveMainURL := cluster.GetMain().URL

	cluster.LogPrintf("INFO :  Main is %s", cluster.GetMain().URL)
	cluster.SetMainStateFailed()
	cluster.SetInteractive(false)
	cluster.GetMain().FailCount = cluster.GetMaxFail()
	cluster.SetFailLimit(3)
	cluster.SetFailTime(0)
	cluster.SetFailoverCtr(3)
	cluster.SetCheckFalsePositiveHeartbeat(false)
	cluster.SetRplChecks(false)
	cluster.SetRplMaxDelay(20)
	cluster.CheckFailed()

	cluster.WaitFailoverEnd()
	cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)
	if cluster.GetMain().URL != SaveMainURL {
		cluster.LogPrintf(LvlErr, "Old main %s ==  Next main %s  ", SaveMainURL, cluster.GetMain().URL)

		SaveMain.FailCount = 0
		return false
	}
	SaveMain.FailCount = 0
	return true
}
