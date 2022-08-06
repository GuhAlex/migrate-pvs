package main

import (

  "fmt"
  "os"
  "log"
  "context"
  "path/filepath"
  "time"
  // appsv1 "k8s.io/api/apps/v1"
  "k8s.io/apimachinery/pkg/types"
  apiv1 "k8s.io/api/core/v1"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  "k8s.io/client-go/tools/clientcmd"
  "k8s.io/client-go/kubernetes"
  "k8s.io/apimachinery/pkg/api/resource"
)

func main () {

  kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
  new_pvc := "new-pvc"

  // bootstrap config
  config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
  if err != nil {
      panic(err.Error())
  }

  // create config
  clientset, err := kubernetes.NewForConfig(config)
  if err != nil {
    log.Fatal(err)
  }

  namespace, workload, size, pvc := get_values()

  work := verify_workload(clientset, workload)

  if work == "deploy" {
    scale_down_deploy(clientset, workload, namespace)
  }
  if work == "statefulset" {
    scale_down_sts(clientset, workload, namespace)
  }
  if work != "deploy" && work != "statefulset" {
    fmt.Println("workload not found")
    os.Exit(1)
  }

  create_pvc(clientset, namespace, size)

  create_datacopy_pod(clientset, namespace, pvc)

  delete_datacopy(clientset, namespace)

  pvc_old, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(context.Background(), pvc, metav1.GetOptions{})
  if err != nil{
  panic(err)
  }
  time.Sleep(10 * time.Second)

  pvc_new, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(context.Background(), new_pvc, metav1.GetOptions{})
  if err != nil{
  panic(err)
  }

  patching_retain(clientset, pvc_new.Spec.VolumeName, pvc_old.Spec.VolumeName, namespace)

  delete_pvcs(clientset, new_pvc, pvc, namespace)

  patch_claimRef(clientset, pvc_new.Spec.VolumeName, pvc_old.Spec.VolumeName, namespace)

  create_new_pvc(clientset, size, pvc, pvc_new.Spec.VolumeName, namespace)

  if work == "deploy" {
    scale_up_deploy(clientset, workload, namespace)
  }
  if work == "statefulset" {
    scale_up_sts(clientset, workload, namespace)
  }
  if work != "deploy" && work != "statefulset" {
    fmt.Println("workload not found")
    os.Exit(1)
  }
}

func scale_down_deploy(api *kubernetes.Clientset, workload string, namespace string){
  s, err := api.AppsV1().Deployments(namespace).GetScale(context.TODO(), workload, metav1.GetOptions{})
  if err != nil {
    log.Fatal(err)
  }

  sc := *s
  sc.Spec.Replicas = 0

  us, err := api.AppsV1().Deployments(namespace).UpdateScale(context.TODO(), workload, &sc, metav1.UpdateOptions{})
  if err != nil {
    log.Fatal(err)
  }
  log.Println(*us)

}

func scale_down_sts(api *kubernetes.Clientset, workload string, namespace string){
  s, err := api.AppsV1().StatefulSets(namespace).GetScale(context.TODO(), workload, metav1.GetOptions{})
  if err != nil {
    log.Fatal(err)
  }

  sc := *s
  sc.Spec.Replicas = 0

  us, err := api.AppsV1().StatefulSets(namespace).UpdateScale(context.TODO(), workload, &sc, metav1.UpdateOptions{})
  if err != nil {
    log.Fatal(err)
  }
  log.Println(*us)

}

func create_datacopy_pod (api *kubernetes.Clientset, namespace string, pvc string) {

  var err error

  pod := &apiv1.Pod {
         ObjectMeta: metav1.ObjectMeta{
               Name:      "datacopy",
               Namespace: namespace,
         },

         Spec: apiv1.PodSpec{
                 Containers: []apiv1.Container{
                   {
                     Name: "datacopy",
                     Image:"ubuntu",
                     Command: []string{
                          "sh",
                          "-c",
                          // "sleep",
                          // "3600",
                          "cd /mnt/old; tar -cf /file.tar.gz . ; tar -xf /file.tar.gz -C /mnt/new/",
                    },
                     VolumeMounts: []apiv1.VolumeMount{
                             apiv1.VolumeMount{
                                  Name:      "old-pv",
                                  MountPath: "/mnt/old",
                             },
                             apiv1.VolumeMount{
                                  Name:      "new-pv",
                                  MountPath: "/mnt/new",
                       },
                     },
                   },
                },
                RestartPolicy: apiv1.RestartPolicyNever,
                 Volumes: []apiv1.Volume{
                      apiv1.Volume{
                        Name: "old-pv",
                        VolumeSource: apiv1.VolumeSource{
                          PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
                            ClaimName: pvc,
                          },
                        },
                      },

                      apiv1.Volume{
                        Name: "new-pv",
                        VolumeSource: apiv1.VolumeSource{
                          PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
                            ClaimName: "new-pvc",
                          },
                        },
                      },
                 },
         },

}
         _, err = api.CoreV1().Pods(namespace).Create(
           context.Background(),
           pod,
           metav1.CreateOptions{},
         )
         if err != nil {
               panic(err)
         }
        fmt.Println("Pod created successfully...")
   }

func create_pvc(api *kubernetes.Clientset, namespace string, size string) {

  storageClassName := "standard"

  pvc := &apiv1.PersistentVolumeClaim{
    ObjectMeta: metav1.ObjectMeta{
      Name: "new-pvc",
      Namespace: namespace,
    },
    Spec: apiv1.PersistentVolumeClaimSpec{
      AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteOnce},
      Resources: apiv1.ResourceRequirements{
         Requests: apiv1.ResourceList{
           apiv1.ResourceName(apiv1.ResourceStorage): resource.MustParse(size),
         },
       },
      StorageClassName: &storageClassName,
    },
  }

  _, err := api.CoreV1().PersistentVolumeClaims(namespace).Create(context.Background(), pvc, metav1.CreateOptions{},)
  if err != nil{
  panic(err)
  }

  fmt.Println("PVC created successfully...")

}

func get_values()(namespace, workload, size, pvc string){
  var err error

  fmt.Print("Namespace:")
  _, err = fmt.Scanln(&namespace)
    if err != nil {
    fmt.Println("error")
  }

  fmt.Print("Workload:")
  _, err = fmt.Scanln(&workload)
    if err != nil {
    fmt.Println("error")
  }

  fmt.Print("Size:")
  _, err = fmt.Scanln(&size)
    if err != nil {
    fmt.Println("error")
  }

  fmt.Print("PVC that will be migrated :")
  _, err = fmt.Scanln(&pvc)
    if err != nil {
    fmt.Println("error")
  }

  return namespace, workload, size, pvc
}

func verify_workload(api *kubernetes.Clientset, workload string)(workloadType string){

  deployment := api.AppsV1().Deployments("")
  sts := api.AppsV1().StatefulSets("")
  listDeploy, err := deployment.List(context.TODO(), metav1.ListOptions{})
  if err != nil{
  panic(err)
  }

  listSts, err := sts.List(context.TODO(), metav1.ListOptions{})
  if err != nil{
  panic(err)
  }


  for _, deployment := range listDeploy.Items {
    if deployment.Name == workload {
      return "deploy"
    }
  }

  for _, sts := range listSts.Items {
    if sts.Name == workload {
      return "statefulset"
    }
  }

 return "workload not found"

}

func patching_retain(api *kubernetes.Clientset, pv_new, pv_old,namespace string){

  var pvs [2]string

  pvs[0] = pv_old
  pvs[1] = pv_new
  // fmt.Println(new_pvc.Spec.VolumeName)

   pv := api.CoreV1().PersistentVolumes()

   retain := `
   [
     { "op": "replace", "path": "/spec/persistentVolumeReclaimPolicy", "value": "Retain" }
   ]
   `

   for _, persistentvolume := range pvs {
       updateRetain, err := pv.Patch(context.Background(), persistentvolume, types.JSONPatchType, []byte(retain), metav1.PatchOptions{})
       if err != nil {
           log.Fatal(err)
       }
       log.Println(updateRetain)
   }
}

func delete_datacopy(api *kubernetes.Clientset, namespace string){

  datacopy := "datacopy"

  for {
      pod, _ := api.CoreV1().Pods(namespace).Get(context.Background(), datacopy, metav1.GetOptions{})
      if pod.Status.Phase == apiv1.PodSucceeded {
        break
      }
    }

    err := api.CoreV1().Pods(namespace).Delete(context.Background(), datacopy, metav1.DeleteOptions{})
    if err != nil{
    panic(err)
    }
}

func delete_pvcs(api *kubernetes.Clientset, pvc_new, pvc_old, namespace string){

  var pvcs [2]string
  pvcs[0] = pvc_new
  pvcs[1] = pvc_old

  for _, persistentvolumeclaim := range pvcs {
    err := api.CoreV1().PersistentVolumeClaims(namespace).Delete(context.Background(), persistentvolumeclaim, metav1.DeleteOptions{})
    if err != nil{
    panic(err)
    }
  }
}

func patch_claimRef(api *kubernetes.Clientset, pv_new, pv_old, namespace string){

  claimref := `
  [
    { "op": "remove", "path": "/spec/claimRef" }
  ]
  `
  var pvs [2]string

  pvs[0] = pv_old
  pvs[1] = pv_new
  pv := api.CoreV1().PersistentVolumes()

  for _, persistentvolume := range pvs {
    updateRetain, err := pv.Patch(context.Background(), persistentvolume, types.JSONPatchType, []byte(claimref), metav1.PatchOptions{})
    if err != nil {
        log.Fatal(err)
    }
    log.Println(updateRetain)
  }
}

func create_new_pvc(api *kubernetes.Clientset, size, pvc, pv_new, namespace string){

  storageClassName := "standard"

  pvc_new := &apiv1.PersistentVolumeClaim{
    ObjectMeta: metav1.ObjectMeta{
      Name: pvc,
      Namespace: namespace,
    },
    Spec: apiv1.PersistentVolumeClaimSpec{
      AccessModes: []apiv1.PersistentVolumeAccessMode{apiv1.ReadWriteOnce},
      Resources: apiv1.ResourceRequirements{
         Requests: apiv1.ResourceList{
           apiv1.ResourceName(apiv1.ResourceStorage): resource.MustParse(size),
         },
       },
      StorageClassName: &storageClassName,
      VolumeName: pv_new,
    },
  }

  time.Sleep(10 * time.Second)


  _, err := api.CoreV1().PersistentVolumeClaims(namespace).Create(context.Background(), pvc_new, metav1.CreateOptions{},)
  if err != nil{
  panic(err)
  }

  fmt.Println("New PVC created successfully...")
}

func scale_up_deploy(api *kubernetes.Clientset, workload, namespace string){
  s, err := api.AppsV1().Deployments(namespace).GetScale(context.TODO(), workload, metav1.GetOptions{})
  if err != nil {
    log.Fatal(err)
  }

  sc := *s
  sc.Spec.Replicas = 1

  us, err := api.AppsV1().Deployments(namespace).UpdateScale(context.TODO(), workload, &sc, metav1.UpdateOptions{})
  if err != nil {
    log.Fatal(err)
  }
  log.Println(*us)
}

func scale_up_sts(api *kubernetes.Clientset, workload, namespace string){
  s, err := api.AppsV1().StatefulSets(namespace).GetScale(context.TODO(), workload, metav1.GetOptions{})
  if err != nil {
    log.Fatal(err)
  }

  sc := *s
  sc.Spec.Replicas = 1

  us, err := api.AppsV1().StatefulSets(namespace).UpdateScale(context.TODO(), workload, &sc, metav1.UpdateOptions{})
  if err != nil {
    log.Fatal(err)
  }
  log.Println(*us)

}
