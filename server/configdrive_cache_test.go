// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigDriveCache", func() {
	Describe("NewConfigDriveCache", func() {
		It("creates a cache with the specified TTL and max size", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			Expect(cache).NotTo(BeNil())
			Expect(cache.Len()).To(Equal(0))
			Expect(cache.Size()).To(Equal(int64(0)))
		})
	})

	Describe("Get and Set", func() {
		It("returns stored data for a valid key", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			data := []byte("test-iso-data")
			cache.Set("key1", data, "rv1")

			result, found := cache.Get("key1")
			Expect(found).To(BeTrue())
			Expect(result).To(Equal(data))
		})

		It("returns false for a non-existent key", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)

			result, found := cache.Get("nonexistent")
			Expect(found).To(BeFalse())
			Expect(result).To(BeNil())
		})

		It("returns false for an expired entry", func() {
			// Use a very short TTL
			cache := NewConfigDriveCache(1*time.Millisecond, 100*1024*1024)
			data := []byte("test-iso-data")
			cache.Set("key1", data, "rv1")

			// Wait for TTL to expire
			time.Sleep(10 * time.Millisecond)

			result, found := cache.Get("key1")
			Expect(found).To(BeFalse())
			Expect(result).To(BeNil())
		})

		It("returns data for a non-expired entry", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			data := []byte("valid-data")
			cache.Set("key1", data, "rv1")

			result, found := cache.Get("key1")
			Expect(found).To(BeTrue())
			Expect(result).To(Equal(data))
		})

		It("overwrites existing entry with same key", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			data1 := []byte("first-data")
			data2 := []byte("second-data-longer")

			cache.Set("key1", data1, "rv1")
			Expect(cache.Size()).To(Equal(int64(len(data1))))

			cache.Set("key1", data2, "rv2")
			Expect(cache.Size()).To(Equal(int64(len(data2))))

			result, found := cache.Get("key1")
			Expect(found).To(BeTrue())
			Expect(result).To(Equal(data2))
		})

		It("tracks size correctly after multiple sets", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			data1 := []byte("data1")
			data2 := []byte("data22")

			cache.Set("key1", data1, "rv1")
			cache.Set("key2", data2, "rv2")

			Expect(cache.Size()).To(Equal(int64(len(data1) + len(data2))))
			Expect(cache.Len()).To(Equal(2))
		})
	})

	Describe("Delete", func() {
		It("removes an existing entry and updates size", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			data := []byte("test-data")
			cache.Set("key1", data, "rv1")

			Expect(cache.Len()).To(Equal(1))
			Expect(cache.Size()).To(Equal(int64(len(data))))

			cache.Delete("key1")

			Expect(cache.Len()).To(Equal(0))
			Expect(cache.Size()).To(Equal(int64(0)))

			_, found := cache.Get("key1")
			Expect(found).To(BeFalse())
		})

		It("does not error when deleting non-existent key", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			// Should not panic or error
			Expect(func() { cache.Delete("nonexistent") }).NotTo(Panic())
			Expect(cache.Len()).To(Equal(0))
			Expect(cache.Size()).To(Equal(int64(0)))
		})

		It("correctly updates size after partial deletion", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			data1 := []byte("data1")
			data2 := []byte("data22")

			cache.Set("key1", data1, "rv1")
			cache.Set("key2", data2, "rv2")

			cache.Delete("key1")

			Expect(cache.Len()).To(Equal(1))
			Expect(cache.Size()).To(Equal(int64(len(data2))))
		})
	})

	Describe("Size eviction", func() {
		It("evicts oldest entry when max size is exceeded", func() {
			// Set max size to 10 bytes
			cache := NewConfigDriveCache(10*time.Minute, 10)
			data1 := []byte("12345") // 5 bytes

			cache.Set("key1", data1, "rv1")
			Expect(cache.Len()).To(Equal(1))

			// Adding another 8-byte entry would exceed 10 byte limit
			data2 := []byte("12345678") // 8 bytes
			cache.Set("key2", data2, "rv2")

			// key1 should have been evicted to make room for key2
			_, found := cache.Get("key1")
			Expect(found).To(BeFalse())

			result, found := cache.Get("key2")
			Expect(found).To(BeTrue())
			Expect(result).To(Equal(data2))
		})

		It("allows an entry that fits within max size", func() {
			cache := NewConfigDriveCache(10*time.Minute, 20)
			data1 := []byte("hello")    // 5 bytes
			data2 := []byte("world12") // 7 bytes - total 12, fits in 20

			cache.Set("key1", data1, "rv1")
			cache.Set("key2", data2, "rv2")

			Expect(cache.Len()).To(Equal(2))
			Expect(cache.Size()).To(Equal(int64(12)))
		})
	})

	Describe("Len and Size", func() {
		It("returns 0 for an empty cache", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			Expect(cache.Len()).To(Equal(0))
			Expect(cache.Size()).To(Equal(int64(0)))
		})

		It("correctly reflects the number of entries", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			for i := 0; i < 5; i++ {
				cache.Set(fmt.Sprintf("key%d", i), []byte("data"), "rv1")
			}
			Expect(cache.Len()).To(Equal(5))
		})
	})

	Describe("concurrent access", func() {
		It("handles concurrent reads and writes safely", func() {
			cache := NewConfigDriveCache(10*time.Minute, 100*1024*1024)
			done := make(chan struct{})

			// Writer goroutine
			go func() {
				for i := 0; i < 100; i++ {
					cache.Set(fmt.Sprintf("key%d", i), []byte("data"), "rv1")
				}
				close(done)
			}()

			// Reader goroutine - just check it doesn't panic
			go func() {
				for i := 0; i < 100; i++ {
					cache.Get(fmt.Sprintf("key%d", i))
				}
			}()

			Eventually(done, "2s").Should(BeClosed())
			Expect(cache.Len()).To(BeNumerically("<=", 100))
		})
	})
})