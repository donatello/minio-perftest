# upload-perftest
Upload performance testing program for Minio Object Storage server.

The command line options are:

```shell
$ ./upload-perftest --help
Usage of ./upload-perftest:
  -bucket string
    	Bucket to use for uploads test (default "bucket")
  -c int
    	concurrency - number of parallel uploads (default 1)
  -h string
    	service endpoint host (default "localhost:9000")
  -m int
    	Maximum amount of disk usage in GBs (default 80)
  -s	Set if endpoint requires https
  -seed int
    	random seed (default 42)

```

Credentials are passed via the environment variables `ACCESS_KEY` and
`SECRET_KEY`.

After the options, a positional parameter for the size of objects to
upload is required. This can be specified with units like `1MiB` or
`1GB`.

The program generates objects of the given size using a fast,
in-memory, partially-random data generator for object content.

The concurrency options sets the number of parallel uploader threads
and simulates multiple uploaders opening separate connections to the
Minio server endpoint. Each thread sequentially performs uploads of
the given size.

The program exits on any kind of upload error with non-zero exit
status. On a successful run, the program exits when uploads have been
continuosly performed for at least 15 minutes and at least 10 objects
have been uploaded.

Every 10 seconds, the program reports the number of objects uploaded,
the average data bandwidth achieved since the start (total object
bytes sent/duration of the test), the average number of objects
uploaded per second since the start, and the total amount of object
data uploaded.

To not overflow disk capacity of the server, the `-m` options takes
the number of GBs of maximum disk space to use in the test. If the
given amount of data is written, the program randomly overwrites
previously written objects.
