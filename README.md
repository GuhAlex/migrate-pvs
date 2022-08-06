## Migrate PVs

This project its a automation based on that [paper](https://www.puzzle.ch/de/blog/articles/2021/08/10/manual-kubernetes-persistentvolumes-migration) to migrate pvs on old K8s using [client-go](https://github.com/kubernetes/client-go) .

### Usage

Here, i'll use mysql deployment, for demo:

```
kubectl apply -f mysql-manifests/
```

Accessing mysql and creating some data:
```
kubectl run -it --rm --image=mysql:5.6 --restart=Never mysql-client -- mysql -h mysql -ppassword
```

```
mysql> CREATE DATABASE bla;
mysql> SHOW DATABASES;
+--------------------+
| Database           |
+--------------------+
| information_schema |
| bla                |
| mysql              |
| performance_schema |
+--------------------+
4 rows in set (0.00 sec)
```

Running _**main.go**_ , an interactive session starts asking for the _Namespace_, _Workload_ name, new PVC size and PVC name:
```
go mod tidy
go run main.go
```

After execution, pv migration will be completed and the old PV will be keep with Available Status
