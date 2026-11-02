package locale

func init() {
	RegisterAll(map[string]map[Lang]string{
		"%w: running intent %d must be stopped before hard deletion": {
			En:     "%w: running intent %d must be stopped before hard deletion",
			Ko:     "%w: 영구 삭제 전에 실행 중인 의도 %d을(를) 중지해야 합니다",
			ZhHans: "%w：永久删除前必须停止正在运行的意图 %d",
			ZhHant: "%w：永久刪除前必須停止正在執行的意圖 %d",
			Ja:     "%w：完全削除する前に実行中のインテント %d を停止してください",
			Es:     "%w: debe detener la intención %d en ejecución antes de la eliminación permanente",
		},
	})
}
