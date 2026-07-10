package infra

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/task/domain"
)

func BenchmarkSQLiteRepositoryFilteredTasks(b *testing.B) {
	repo := newSQLiteRepositoryForBenchmark(b, 10_000)
	filter := domain.TaskFilter{
		ProjectID: types.ProjectID("project-05"),
		Status:    []domain.TaskStatus{domain.TaskStatusActive},
		Limit:     25,
		Offset:    50,
		SortBy:    "updated_at",
		SortDesc:  true,
	}
	ctx := context.Background()
	matched, err := repo.CountTasks(ctx, filter)
	if err != nil {
		b.Fatal(err)
	}
	if matched != 500 {
		b.Fatalf("benchmark fixture matched %d tasks, want 500", matched)
	}

	b.Run("list", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := repo.ListTasks(ctx, filter); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("count", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := repo.CountTasks(ctx, filter); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func newSQLiteRepositoryForBenchmark(b *testing.B, taskCount int) *SQLiteRepository {
	b.Helper()
	repo := newSQLiteRepositoryForTest(b)
	ctx := context.Background()
	statuses := []domain.TaskStatus{
		domain.TaskStatusPending,
		domain.TaskStatusActive,
		domain.TaskStatusInReview,
		domain.TaskStatusSucceeded,
	}
	for i := range taskCount {
		task := infraTask(
			fmt.Sprintf("task-%05d", i),
			fmt.Sprintf("project-%02d", i%20),
			statuses[i%len(statuses)],
		)
		createdAt := infraTestNow.Add(time.Duration(i) * time.Second)
		updatedAt := createdAt.Add(time.Duration(i%300) * time.Second)
		task.CreatedAt = &createdAt
		task.UpdatedAt = &updatedAt
		if err := repo.CreateTask(ctx, task); err != nil {
			b.Fatalf("create benchmark task %d: %v", i, err)
		}
	}
	return repo
}
