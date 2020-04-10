package engine

func (engine *WebEngine) registerRouter() {
	group := engine.router.Group(engine.prefix)
	{
		group.GET("/ws", engine.handle.Connect)
	}
}
