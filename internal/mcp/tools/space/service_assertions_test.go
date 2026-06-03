package space

import spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"

var _ SpaceService = (*spaceapp.Service)(nil)
var _ MemberService = (*spaceapp.Service)(nil)
var _ MemberRegistrar = (*spaceapp.Service)(nil)
