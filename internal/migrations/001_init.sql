-- ============================================================
-- 企业知识库问答系统 — 数据库初始化迁移
-- 版本: 001
-- 说明: 创建所有业务表、索引、pgvector 扩展
-- 执行方式: Docker Compose 启动时自动执行
--   （因为 migrations/ 目录挂载到 /docker-entrypoint-initdb.d/）
-- ============================================================

-- 启用 pgvector 扩展(提供 vector 数据类型和相似度运算符)
-- <=> 运算符表示余弦距离,1 - (a <=> b) 得到余弦相似度
CREATE EXTENSION IF NOT EXISTS vector;

-- ============================================================
-- 1. 知识库表
-- 一个知识库包含多份文档，是文档和对话的组织单元
-- ============================================================
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),  -- UUID 主键,全局唯一
    name TEXT NOT NULL,                             -- 知识库名称
    description TEXT NOT NULL DEFAULT '',           -- 描述(可选)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),  -- 创建时间(带时区)
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()   -- 最后更新时间
);

-- ============================================================
-- 2. 文档表
-- 记录上传的原始文档信息，不存储文件内容（文件存在磁盘上）
-- status 字段追踪异步处理状态：pending → parsing → chunking → embedding → completed/failed
-- ============================================================
CREATE TABLE IF NOT EXISTS documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,   -- 级联删除
    file_name TEXT NOT NULL,                -- 原始文件名 (如: 员工手册.pdf)
    file_type TEXT NOT NULL,                -- 文件类型: pdf, docx, md, txt
    file_size BIGINT NOT NULL DEFAULT 0,    -- 文件大小 (字节)
    status TEXT NOT NULL DEFAULT 'pending',  -- 处理状态
    error_message TEXT,                     -- 处理失败时的错误信息
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- status 状态流转:
-- pending    → 刚上传，等待处理
-- parsing    → 正在解析文档提取文本
-- chunking   → 正在将文本切成小块
-- embedding  → 正在调 API 生成向量
-- completed  → 处理完成，可以检索
-- failed     → 处理失败，查看 error_message

-- 按知识库查询文档列表 (最常用的查询)
CREATE INDEX idx_documents_kb_id ON documents(kb_id);

-- 按状态过滤 (如查询处理中的文档)
CREATE INDEX idx_documents_status ON documents(status);

-- ============================================================
-- 3. 切片表（核心表）
-- 每个文档被切分成多个 chunk，每个 chunk 存储文本内容和向量
-- 这是 RAG 检索的关键：用户问题向量化后，在这张表里找最相似的片段
-- ============================================================
CREATE TABLE IF NOT EXISTS chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doc_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    kb_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,   -- 冗余字段，避免 JOIN documents 才能按知识库检索
    chunk_index INT NOT NULL,                           -- 切片在原文档中的序号（从 0 开始）
    content TEXT NOT NULL,                              -- 切片文本内容
    token_count INT NOT NULL DEFAULT 0,                 -- 估算的 token 数量
    embedding vector(1536),                             -- pgvector 向量字段（1536 维，对应 OpenAI embedding 模型）
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 按文档 ID 查询所有切片
CREATE INDEX idx_doc_id ON chunks(doc_id);
-- 按知识库 ID 过滤 (向量检索时需要限定知识库范围)
CREATE INDEX idx_kb_id ON chunks(kb_id);
-- 向量相似度索引（核心性能索引）
-- IVFFlat: 将向量空间划分为 N 个区域（lists），检索时只搜索最近的几个区域
-- vector_cosine_ops: 使用余弦距离算子（<=>）
-- lists = 100: 经验值，建议在 100~1000 之间，数据量大时适当增加
CREATE INDEX idx_chunks_embedding ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- ============================================================
-- 4. 对话表
-- 每次用户在一个知识库下开始对话创建一个 conversation
-- ============================================================
CREATE table IF NOT EXISTS conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '新对话',          -- 对话标题,首条问题截取
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()   -- 最后一条消息时间
);

CREATE INDEX idx_conversations_kb_id ON conversations(kb_id);

-- ============================================================
-- 5. 消息表
-- 记录对话中的每条消息（用户提问 + 助手回答）
-- sources 用 JSONB 存储引用的原文片段，灵活可扩展
-- ============================================================
CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL,       -- user（用户提问） 或 assistant（助手回答）
    content TEXT NOT NULL,    -- 消息内容
    sources JSONB,            -- 引用的来源：[{"content":"...","doc":"员工手册.pdf","score":0.92}]
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 按对话 ID + 时间排序获取消息历史（最常用查询）
CREATE INDEX idx_messages_conversation ON messages(conversation_id, created_at DESC );