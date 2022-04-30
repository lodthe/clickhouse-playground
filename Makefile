.PHONY: docker-publish-x86

docker-publish-x86:
	docker buildx build --platform linux/x86_64 -t lodthe/clickhouse-playground .
	docker push lodthe/clickhouse-playground
	@echo "Server image published"