// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import "github.com/signal18/replication-manager/cluster"

func testFailoverAllSubordinatesDelayNoRplChecksNoSemiSync(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)

		return false
	}
	SaveMainURL := cluster.GetMain().URL
	cluster.LogPrintf("TEST", "Main is %s", cluster.GetMain().URL)
	cluster.SetInteractive(false)
	cluster.SetFailoverCtr(0)
	cluster.SetCheckFalsePositiveHeartbeat(false)
	cluster.SetRplChecks(false)
	cluster.SetRplMaxDelay(4)

	cluster.DelayAllSubordinates()

	cluster.FailoverAndWait()

	cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)

	if cluster.GetMain().URL == SaveMainURL {
		cluster.LogPrintf(LvlErr, "Old main %s ==  New main %s  ", SaveMainURL, cluster.GetMain().URL)

		return false
	}

	return true
}
