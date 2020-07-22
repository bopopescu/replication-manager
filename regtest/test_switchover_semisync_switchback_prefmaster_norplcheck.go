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

func testSwitchoverBackPreferedMainNoRplCheckSemiSync(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetRplChecks(false)
	cluster.SetRplMaxDelay(0)
	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)
		return false
	}
	cluster.SetPrefMain(cluster.GetMain().URL)
	cluster.LogPrintf("TEST", "Set cluster.conf.PrefMain %s", "cluster.conf.PrefMain")
	time.Sleep(2 * time.Second)
	SaveMainURL := cluster.GetMain().URL
	for i := 0; i < 2; i++ {
		cluster.LogPrintf("TEST", "New Main  %s Failover counter %d", cluster.GetMain().URL, i)
		cluster.SwitchoverWaitTest()
		cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)
	}
	if cluster.GetMain().URL != SaveMainURL {
		cluster.LogPrintf(LvlErr, "Saved Prefered main %s <>  from saved %s  ", SaveMainURL, cluster.GetMain().URL)
		return false
	}
	return true
}
