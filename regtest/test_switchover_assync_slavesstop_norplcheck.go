// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import (
	"time"

	"github.com/signal18/replication-manager/cluster"
)

func testSwitchoverAllSubordinatesStopNoSemiSyncNoRplCheck(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetRplMaxDelay(0)
	cluster.SetRplChecks(false)
	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)
		return false
	}
	err = cluster.StopSubordinates()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)
		return false
	}
	time.Sleep(4 * time.Second)
	SaveMainURL := cluster.GetMain().URL
	cluster.LogPrintf("TEST", "Main is %s", cluster.GetMain().URL)

	cluster.SwitchoverWaitTest()
	cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)
	time.Sleep(2 * time.Second)
	if cluster.GetMain().URL == SaveMainURL {
		cluster.LogPrintf(LvlErr, "Saved Prefered main %s <>  from saved %s  ", SaveMainURL, cluster.GetMain().URL)
		return false
	}
	return true
}
