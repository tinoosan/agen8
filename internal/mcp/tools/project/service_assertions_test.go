package project

import projectapp "github.com/tinoosan/agen8/internal/services/project/app"

var _ MemberService = (*projectapp.Service)(nil)
var _ MemberRegistrar = (*projectapp.Service)(nil)
