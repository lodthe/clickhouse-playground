# Playground API specification

Users interact with the platform via the exposed HTTP API.

The project is in the beta stage, so 
backward compatibility may be broken (in extreme situations).

## Authorization and authentication

---

There are no auth mechanisms at the moment. You are not required to 
provide a token, credentials or something else to send a request. 
There are plans to integrate SSO-based auth.

## Response structure

---

All responses have the following JSON structure:

```yml
{
  [optional] error: {
    message: string,
    code: int
  },
  [optional] result: {
    // payload
  }
}
```

Responses have either `error` or `result` fields that describe 
an occurred error or contains payload respectively.

Examples:
```yml
# Error:
{
  "error": {
    "message": "invalid ClickHouse version",
    "code": 400
  }
}

# Success:
{
  "result": {
    "query_run_id":"612e2b9e-12db-4644-a933-d0693a15ecb5",
    "output":"1\n",
    "time_elapsed":"1.069s"
  }
}
```

If a response payload is presented, the request has been processed 
correctly and the status code is 200.

## Endpoints

---

The base URI is `https://playground.lodthe.me/`.

### List available ClickHouse versions

| GET    | /api/tags |
|--------|-----------|

Get available ClickHouse versions that can be used for running a query.
Returned versions are just DockerHub image tags.

<details>
    <summary>Response payload</summary>
    <table>
        <thead>
            <tr>
                <th>Field name</th>
                <th>Field type</th>
                <th>Description</th>
            </tr>
        </thead>
        <tbody>
            <tr>
                <td rowspan=1>tags</td>
                <td rowspan=1>array[string]</td>
                <td>List of available ClickHouse versions (tags).</td>
            </tr>
        </tbody>
    </table>
</details>

Example:
```yml
curl -XGET https://playground.lodthe.me/api/tags

# 200 OK
{
  "result": {
    "tags": [
      "head",
      "22.5.1", 
      "22.5.1-alpine", 
      ..., 
      "19.8"
    ]
   }
}
```

### Run a query

| POST   | /api/runs |
|--------|-----------|

The title speaks for itself.  Keep in mind, a new container is created 
for an incoming request, so it may some time to process the query 
(15 &ndash; 20 seconds for absent images).

<details>
    <summary>Request body</summary>
    <table>
        <thead>
            <tr>
                <th>Field name</th>
                <th>Field type</th>
                <th>Description</th>
            </tr>
        </thead>
        <tbody>
            <tr>
                <td rowspan=1>version</td>
                <td rowspan=1>string</td>
                <td>A desired version of ClickHouse where the query will be run.</td>
            </tr>
            <tr>
                <td rowspan=1>input</td>
                <td rowspan=1>string</td>
                <td>Semicolon-separated list of SQL queries that will be run.</td>
            </tr>
        </tbody>
    </table>
</details>

<details>
    <summary>Response payload</summary>
    <table>
        <thead>
            <tr>
                <th>Field name</th>
                <th>Field type</th>
                <th>Description</th>
            </tr>
        </thead>
        <tbody>
            <tr>
                <td rowspan=1>query_run_id</td>
                <td rowspan=1>string</td>
                <td>May be used to get the query run details.</td>
            </tr>
            <tr>
                <td>output</td>
                <td>string</td>
                <td>Query run execution result.</td>
            </tr>
            <tr>
                <td>time_elapsed</td>
                <td>string</td>
                <td>How long it took to process the query on the server side.</td>
            </tr>
        </tbody>
    </table>
</details>

Example:
```yml
curl -XPOST https://playground.lodthe.me/api/runs -d '{ \
  "version": "22.5.1", \
  "query": "SELECT * FROM numbers(0, 5)" \
}'

# 200 OK
{
  "result": {
    "query_run_id": "1bcb005d-f466-4036-a5e3-81c723096913",
    "output":"0\n1\n2\n3\n4\n",
    "time_elapsed":"1.069s"
  }
}
```

### Get a query execution result


| GET    | /api/runs/{query_run_id} |
|--------|--------------------------|

You can get information about a previously processed query.

<details>
    <summary>Endpoint parameters</summary>
    <table>
        <thead>
            <tr>
                <th>Field name</th>
                <th>Description</th>
            </tr>
        </thead>
        <tbody>
            <tr>
                <td rowspan=1>query_run_id</td>
                <td>ID of a finished query run.</td>
            </tr>
        </tbody>
    </table>
</details>

<details>
    <summary>Response payload</summary>
    <table>
        <thead>
            <tr>
                <th>Field name</th>
                <th>Field type</th>
                <th>Description</th>
            </tr>
        </thead>
        <tbody>
            <tr>
                <td rowspan=1>query_run_id</td>
                <td rowspan=1>string</td>
                <td>ID of the finished query run.</td>
            </tr>
            <tr>
                <td rowspan=1>version</td>
                <td rowspan=1>string</td>
                <td>What ClickHouse version has been used to run the query.</td>
            </tr>
            <tr>
                <td>input</td>
                <td>string</td>
                <td>Provided queries.</td>
            </tr>
            <tr>
                <td>output</td>
                <td>string</td>
                <td>Query run execution result.</td>
            </tr>
        </tbody>
    </table>
</details>

Example:
```yml
curl -XGET https://playground.lodthe.me/api/runs/1bcb005d-f466-4036-a5e3-81c723096913

# 200 OK
{
  "result": {
    "query_run_id": "1bcb005d-f466-4036-a5e3-81c723096913",
    "version": "latest",
    "input": "select * from numbers(0, 5)",
    "output": "0\n1\n2\n3\n4\n"
  }
}
```