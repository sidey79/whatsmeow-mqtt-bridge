package whatsapp

func (c *Client) eventHandler() EventHandler {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handler
}

func (c *Client) messageWorker() {
	for {
		select {
		case evt := <-c.messages:
			if handler := c.eventHandler(); handler != nil {
				handler.OnMessage(evt)
			}
		case <-c.eventCtx.Done():
			return
		}
	}
}
