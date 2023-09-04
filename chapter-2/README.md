# Chapter 2 :: Cloud-Native Application Challenges

In this short tutorial we will be installing the `Conference Application` using Helm into a local KinD Kubernetes Cluster. 

Helm Charts can be published to Helm Chart repositories or also, since Helm 3.7 as OCI containers to container registries. 

Check the pre-requisites for all the totutorials [here](../chapter-1/README.md#pre-requisites-for-the-other-chapters)

## Creating a local cluster with Kubernetes KinD

Create a KinD Cluster with 3 worker nodes and 1 Control Plane

```
cat <<EOF | kind create cluster --name dev --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
- role: worker
- role: worker
- role: worker
EOF

```

![3 worker nodes](imgs/cluster-topology.png)

### Loading some container images before installing the application and other components

The `kind-load.sh` script prefetch (pulls and load container images) that we will be using for our application to our KinD Cluster. 
The idea here is to optimize the process for our Cluster, so when we install the application, we don't wait for 10+ minutes to fetch all the container images needed. With all images already pre-loaded into our KinD cluster, the application should start in around 1 minute, as we still need to wait for PostgreSQL, Redis and Kafka to bootstrap. 

Run the script by copying this command into your terminal, from inside the `chapter-2` directory: 

```
./kind-load.sh
```

By running this script you will be fetching all the required images and then loading them into every node of your KinD cluster. If you are running the examples on a Cloud Provider, this might not be worth as Cloud Providers with Gigabyte connections to container registries might fetch these iamges in matter of seconds.



### Installing NGINX Ingress Controller

We need NGINGX Ingress Controller to route traffic from our laptop to the services that are running inside the cluster. NGINX Ingress Controller act as a router that is running inside the cluster, but exposed to the outside world. 

```
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/release-1.8/deploy/static/provider/kind/deploy.yaml
```

Check that the pods inside the `ingress-nginx` are started correctly before proceeding: 
```
> kubectl get pods -n ingress-nginx
NAME                                        READY   STATUS      RESTARTS   AGE
ingress-nginx-admission-create-cflcl        0/1     Completed   0          62s
ingress-nginx-admission-patch-sb64q         0/1     Completed   0          62s
ingress-nginx-controller-5bb6b499dc-7chfm   0/1     Running     0          62s
```

This allows you to route traffic from http://localhost to services running inside the cluster. Notice that for KinD to work in this way, when we created the cluster we provided extra parameters and labels for the control plane node:
```
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true" #This allow the ingress controller to be installed in the control plane node
  extraPortMappings:
  - containerPort: 80 # This allows us to bind port 80 in local host to the ingress controller, so it can route traffic to services running inside the cluster.
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
```

Once we have our cluster and our Ingress Controller installed and configured we can move ahead and install our application.


## Installing the Conference Application

From Helm 3.7+, we can use OCI images to publish, download and install Helm Charts. This approach uses Docker Hub as a Helm Chart registry and to install the Conference Application you only need to run the following command:

```
helm install conference oci://docker.io/salaboy/conference-app --version v1.0.0
```

You can also run the following command to see the details of the chart: 

```
helm show all oci://docker.io/salaboy/conference-app --version v1.0.0
```

Check that all the application pods are up and running. Notice that if your internet connection is slow it might take a while for the application to start. Since the application's services depend on some infrastructure components (Redis, Kafka, PostgreSQL), these components needs to start and be ready for the services to connect. Components like Kakfa are quite heavy with around 335+ MB, PostgreSQL 88+ MB, and Redis 35+ MB.

Eventually you should see something like this, it can take a few minutes: 

```
kubect get pods
NAME                                                           READY   STATUS    RESTARTS      AGE
conference-agenda-service-deployment-7cc9f58875-k7s2x          1/1     Running   4 (45s ago)   2m2s
conference-c4p-service-deployment-54f754b67c-br9dg             1/1     Running   4 (65s ago)   2m2s
conference-frontend-deployment-74cf86495-jthgr                 1/1     Running   4 (56s ago)   2m2s
conference-kafka-0                                             1/1     Running   0             2m2s
conference-notifications-service-deployment-7cbcb8677b-rz8bf   1/1     Running   4 (47s ago)   2m2s
conference-postgresql-0                                        1/1     Running   0             2m2s
conference-redis-master-0                                      1/1     Running   0             2m2s
```

The Pod restarts show that maybe Kafka was slow and the service was started first by Kubernetes, hence it needed to be restarted to wait for Kafka to be ready. 


Now you can point your browser to [http://localhost](http://localhost) to see the application. 

![conference app](imgs/conference-app-homepage.png)

## Clean up - Important !!! READ!!

Because the Conference Application is installing PostgreSQL, Redis and Kafka, if you want to remove and install the application again you need to make sure to delete the associated PersistenceVolumeClaims (PVCs). These PVCs are the volumes uses to store the data from the databases and Kafka, failing to delete these PVCs in between installations will cause the services to use old credentials to connect to the new provisioned databases. 

You can delete all PVCs by listing them with:

```
kubectl get pvc
```

You should see:

```
NAME                                   STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
data-conference-kafka-0                Bound    pvc-2c3ccdbe-a3a5-4ef1-a69a-2b1022818278   8Gi        RWO            standard       8m13s
data-conference-postgresql-0           Bound    pvc-efd1a785-e363-462d-8447-3e48c768ae33   8Gi        RWO            standard       8m13s
redis-data-conference-redis-master-0   Bound    pvc-5c2a96b1-b545-426d-b800-b8c71d073ca0   8Gi        RWO            standard       8m13s
```

And then delete with: 
```
kubectl delete pvc  data-conference-kafka-0 data-conference-postgresql-0 redis-data-conference-redis-master-0
```

The name of the PVCs will change based on the Helm Release name that you used when installing the chart.

Finally, if you want to get rid of the KinD Cluster entirely, you can run:

```
kind delete clusters dev
```


## Next Steps

I strongly recommend you to get your hands dirty with a real Kubernetes Cluster hosted in a Cloud Provider. You can try most Cloud Providers, as they offer a free trial where you can create Kubernetes Clusters and run all these examples, [check this repository for more information](https://github.com/learnk8s/free-kubernetes). 

If you can create a Cluster in a Cloud provider and get the application up and running you will gain real life experience on all the topics covered in Chapter 2.

## Sum up and Contribute

In this short tutorial we have managed to install the Conference Application walking skeleton. We will be using this application as an example through the rest of the chapters. Make sure that this application works for you as it cover the basics of using and interacting with a Kubernetes Cluster. 


Do you want to improve this tutorial? Create an issue, drop me a message on [Twitter](https://twitter.com/salaboy) or send a Pull Request.