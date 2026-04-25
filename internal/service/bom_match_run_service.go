package service

import (
	"context"

	v1 "caichip/api/bom/v1"
)

func (s *BomService) SaveMatchRun(ctx context.Context, req *v1.SaveMatchRunRequest) (*v1.SaveMatchRunReply, error) {
	return nil, notImplemented("SaveMatchRun 未实现")
}

func (s *BomService) ListMatchRuns(ctx context.Context, req *v1.ListMatchRunsRequest) (*v1.ListMatchRunsReply, error) {
	return nil, notImplemented("ListMatchRuns 未实现")
}

func (s *BomService) GetMatchRun(ctx context.Context, req *v1.GetMatchRunRequest) (*v1.GetMatchRunReply, error) {
	return nil, notImplemented("GetMatchRun 未实现")
}
