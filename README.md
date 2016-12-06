# minio-perftest
Performance testing program(s) for Minio Object Storage server.

```shell
$ ./minio-perftest --help
Usage of ./minio-perftest:
  -bucket string
    	Bucket to use for uploads test (default "bucket")
  -c int
    	concurrency - number of parallel uploads (default 1)
  -h string
    	service endpoint host (default "localhost:9000")
  -s	Set if endpoint requires https
  -seed int
    	random seed (default 42)

```
