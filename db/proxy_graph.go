package db

import "github.com/Autumn-27/artex/proxygraph"

func (d *DB) ProxyGraphNodes() ([]proxygraph.Proxy, error) {
	rows, err := d.Query(`SELECT id, url, kind, asn, active, score FROM proxy_graph_nodes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []proxygraph.Proxy{}
	for rows.Next() {
		var n proxygraph.Proxy
		if err := rows.Scan(&n.ID, &n.URL, &n.Type, &n.ASN, &n.Active, &n.Score); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (d *DB) SaveProxyGraphNode(n proxygraph.Proxy) error {
	_, err := d.Exec(`INSERT INTO proxy_graph_nodes(id,url,kind,asn,active,score)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(id) DO UPDATE
		SET url=excluded.url, kind=excluded.kind, asn=excluded.asn,
		active=excluded.active, score=excluded.score`, n.ID, n.URL, n.Type, n.ASN, n.Active, n.Score)
	return err
}

func (d *DB) DeleteProxyGraphNode(id string) error {
	_, err := d.Exec(`DELETE FROM proxy_graph_nodes WHERE id=$1`, id)
	return err
}

func (d *DB) UpdateProxyGraphHealth(n proxygraph.Proxy) error {
	_, err := d.Exec(`UPDATE proxy_graph_nodes SET active=$3,score=$4,last_checked=now()
		WHERE id=$1 AND url=$2`, n.ID, n.URL, n.Active, n.Score)
	return err
}
