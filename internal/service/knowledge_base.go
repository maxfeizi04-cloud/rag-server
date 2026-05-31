package service

import (
	"context"
	"fmt"
	"rag-server/internal/model"
	"rag-server/internal/repository"
)

// KnowledgeBaseService 知识库业务逻辑层
// 在 Repository 之上封装业务规则和校验
type KnowledgeBaseService struct {
	kbRepo *repository.KnowledgeBaseRepo
}

func NewKnowledgeBaseService(kbRepo *repository.KnowledgeBaseRepo) *KnowledgeBaseService {
	return &KnowledgeBaseService{kbRepo: kbRepo}
}

// Create 创建知识库
// 业务规则：
//  1. 名称不能为空
//  2. 描述可选，默认空字符串
func (s *KnowledgeBaseService) Create(ctx context.Context, req model.CreateKBRequest) (*model.KnowledgeBase, error) {
	// 参数校验
	if req.Name == "" {
		return nil, fmt.Errorf("知识库名称不能为空")
	}
	return s.kbRepo.Create(ctx, req.Name, req.Description)
}

// Get 获取知识库详情
// 不存在时返回明确的错误信息
func (s *KnowledgeBaseService) Get(ctx context.Context, id string) (*model.KnowledgeBase, error) {
	kb, err := s.kbRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if kb == nil {
		return nil, fmt.Errorf("知识库不存在")
	}
	return kb, nil
}

// List 分页查询知识库列表
// page/pageSize 参数做边界校验
func (s *KnowledgeBaseService) List(ctx context.Context, page, pageSize int) ([]model.KnowledgeBase, int64, error) {
	// 参数净化：页码最小 1，每页最多 100 条
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.kbRepo.List(ctx, page, pageSize)
}

// Update 更新知识库
func (s *KnowledgeBaseService) Update(ctx context.Context, id string, req model.UpdateKBRequest) (*model.KnowledgeBase, error) {
	kb, err := s.kbRepo.Update(ctx, id, req.Name, req.Description)
	if err != nil {
		return nil, err
	}
	if kb == nil {
		return nil, fmt.Errorf("知识库不存在")
	}
	return kb, nil
}

// Delete 删除知识库
// 先检查存在性，再执行删除（友好报错）
func (s *KnowledgeBaseService) Delete(ctx context.Context, id string) error {
	kb, err := s.kbRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if kb == nil {
		return fmt.Errorf("知识库不存在")
	}
	return s.kbRepo.Delete(ctx, id)
}
