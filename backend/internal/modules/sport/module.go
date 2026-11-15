package sport

import "athlix/backend/internal/app"

// Module is the sport module. See README.md in this folder.
type Module struct{ d *app.Deps }

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d} }

// Name implements app.Module.
func (m *Module) Name() string { return "sport" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{d: m.d}
	r.Public.GET("/sports", h.list)
}
