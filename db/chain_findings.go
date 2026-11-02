package db

type ChainFinding struct {
	ID        int64  `json:"id"`
	VulnClass string `json:"vulnclass"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
}

func (d *DB) ConfirmedChainFindings(taskID int64) ([]ChainFinding, error) {
	rows, err := d.Query(`SELECT id, left(vulnclass,80), left(severity,30), left(summary,400)
		FROM findings WHERE task_id=$1 AND status='confirmed' ORDER BY id DESC LIMIT 50`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ChainFinding, 0)
	for rows.Next() {
		var finding ChainFinding
		if err := rows.Scan(&finding.ID, &finding.VulnClass, &finding.Severity, &finding.Summary); err != nil {
			return nil, err
		}
		out = append(out, finding)
	}
	return out, rows.Err()
}

func (s *ExplorationStore) ConfirmedChainFindings(taskID int64) ([]ChainFinding, error) {
	return s.db.ConfirmedChainFindings(taskID)
}
