// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package cluster

import (
	"errors"
)

func (cluster *Cluster) RollingReprov() error {

	cluster.LogPrintf(LvlInfo, "Rolling reprovisionning")
	mainID := cluster.GetMain().Id
	for _, subordinate := range cluster.subordinates {
		if !subordinate.IsDown() {
			if !subordinate.IsMaintenance {
				subordinate.SwitchMaintenance()
			}
			err := cluster.UnprovisionDatabaseService(subordinate)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Cancel rolling reprov %s", err)
				return err
			}
			err = cluster.WaitDatabaseFailed(subordinate)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Cancel rolling restart subordinate does not transit suspect %s %s", subordinate.URL, err)
				return err
			}
			err = cluster.InitDatabaseService(subordinate)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Cancel rolling reprov %s", err)
				return err
			}
			err = cluster.StartDatabaseWaitRejoin(subordinate)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Cancel rolling reprov %s", err)
				return err
			}

			subordinate.WaitSyncToMain(cluster.main)
			subordinate.SwitchMaintenance()
		}
	}
	cluster.SwitchoverWaitTest()
	main := cluster.GetServerFromName(mainID)
	if cluster.main.DSN == main.DSN {
		cluster.LogPrintf(LvlErr, "Cancel rolling restart main is the same after Switchover")
		return nil
	}
	if !main.IsDown() {
		if !main.IsMaintenance {
			main.SwitchMaintenance()
		}
		err := cluster.UnprovisionDatabaseService(main)
		if err != nil {
			cluster.LogPrintf(LvlErr, "Cancel rolling reprov %s", err)
			return err
		}
		err = cluster.WaitDatabaseFailed(main)
		if err != nil {
			cluster.LogPrintf(LvlErr, "Cancel rolling restart subordinate does not transit suspect %s %s", main.URL, err)
			return err
		}
		err = cluster.InitDatabaseService(main)
		if err != nil {
			cluster.LogPrintf(LvlErr, "Cancel rolling reprov %s", err)
			return err
		}
		err = cluster.WaitDatabaseStart(main)
		if err != nil {
			cluster.LogPrintf(LvlErr, "Cancel rolling reprov %s", err)
			return err
		}
		main.WaitSyncToMain(cluster.main)
		main.SwitchMaintenance()
		cluster.SwitchOver()
	}
	return nil
}

func (cluster *Cluster) RollingRestart() error {
	cluster.LogPrintf(LvlInfo, "Rolling restart")
	mainID := cluster.GetMain().Id
	saveFailoverMode := cluster.Conf.FailSync
	cluster.SetFailSync(false)
	defer cluster.SetFailSync(saveFailoverMode)
	for _, subordinate := range cluster.subordinates {

		if !subordinate.IsDown() {
			//subordinate.SetMaintenance()
			//proxy.
			if !subordinate.IsMaintenance {
				subordinate.SwitchMaintenance()
			}
			err := cluster.StopDatabaseService(subordinate)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Cancel rolling restart stop failed on subordinate %s %s", subordinate.URL, err)
				return err
			}

			err = cluster.WaitDatabaseFailed(subordinate)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Cancel rolling restart subordinate does not transit suspect %s %s", subordinate.URL, err)
				return err
			}

			err = cluster.StartDatabaseWaitRejoin(subordinate)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Cancel rolling restart subordinate does not restart %s %s", subordinate.URL, err)
				return err
			}
		}
		subordinate.WaitSyncToMain(cluster.main)
		subordinate.SwitchMaintenance()
	}
	cluster.SwitchoverWaitTest()
	main := cluster.GetServerFromName(mainID)
	if cluster.main.DSN == main.DSN {
		cluster.LogPrintf(LvlErr, "Cancel rolling restart main is the same after Switchover")
		return nil
	}
	if main.IsDown() {
		return errors.New("Cancel roolling restart main down")
	}
	if !main.IsMaintenance {
		main.SwitchMaintenance()
	}
	err := cluster.StopDatabaseService(main)
	if err != nil {
		cluster.LogPrintf(LvlErr, "Cancel rolling restart old main stop failed %s %s", main.URL, err)
		return err
	}
	err = cluster.WaitDatabaseFailed(main)
	if err != nil {
		cluster.LogPrintf(LvlErr, "Cancel rolling restart old main does not transit suspect %s %s", main.URL, err)
		return err
	}
	err = cluster.StartDatabaseWaitRejoin(main)
	if err != nil {
		cluster.LogPrintf(LvlErr, "Cancel rolling restart old main does not restart %s %s", main.URL, err)
		return err
	}
	main.WaitSyncToMain(cluster.main)
	main.SwitchMaintenance()
	cluster.SwitchOver()

	return nil
}

func (cluster *Cluster) RollingOptimize() {
	for _, s := range cluster.subordinates {
		jobid, _ := s.JobOptimize()
		cluster.LogPrintf(LvlInfo, "Optimize job id %d on %s ", jobid, s.URL)
	}
}
