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

func testSwitchoverLongQueryNoRplCheckNoSemiSync(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetRplChecks(false)
	cluster.SetRplMaxDelay(8)

	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)
		return false
	}

	SaveMainURL := cluster.GetMain().URL
	go dbhelper.InjectLongTrx(cluster.GetMain().Conn, 20)

	cluster.LogPrintf("TEST", "Main is %s", cluster.GetMain().URL)
	cluster.SwitchoverWaitTest()
	cluster.LogPrintf("TEST", "New Main  %s ", cluster.GetMain().URL)

	time.Sleep(20 * time.Second)
	err = cluster.EnableSemisync()
	if err != nil {
		cluster.LogPrintf(LvlErr, "%s", err)
		return false
	}
	if cluster.GetMain().URL != SaveMainURL {
		cluster.LogPrintf(LvlErr, "Saved Prefered main %s <>  from saved %s  ", SaveMainURL, cluster.GetMain().URL)
		return false
	}
	return true
}
