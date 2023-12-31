package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"path/filepath"

	// "github.com/google/uuid"

	// "k8s.io/apimachinery/pkg/api/errors"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {

	clientset := getClientSet()

	initCluster(clientset)
	// jobId := uuid.New()

	// createJob(clientset, jobId.String(), "ls")

    wg := sync.WaitGroup{}
    wg.Add(1)
	go getJobLogs(clientset, "example-job", os.Stdout, &wg)
    wg.Wait()
}

func getClientSet() *kubernetes.Clientset {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

func printPods(clientset *kubernetes.Clientset) {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	for _, pod := range pods.Items {
		fmt.Printf("Pod : %-60s\t Namespace : %s\n", pod.Name, pod.Namespace)
	}
}

func initCluster(clientset *kubernetes.Clientset) {
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	found := false
	for _, namespace := range namespaces.Items {
		if namespace.Name == "scheduler" {
			found = true
			break
		}
	}
	if !found {
		newNamespace := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scheduler",
			},
		}
		_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), newNamespace, metav1.CreateOptions{})
		if err != nil {
			panic(err.Error())
		}
	}
}

func printJobs(clientset *kubernetes.Clientset) {
	jobs, err := clientset.BatchV1().Jobs("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d jobs in the cluster\n", len(jobs.Items))

	for _, job := range jobs.Items {
		fmt.Printf("Pod : %-60s\t Namespace : %s\n", job.Name, job.Namespace)
	}
}

func createJob(clientset *kubernetes.Clientset, name string, command string) {
	var backoffLimit int32 = 0
	newJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "scheduler",
		},
		Spec: batchv1.JobSpec{
			Template: apiv1.PodTemplateSpec{
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:    name,
							Image:   "jonlabelle/network-tools:latest",
							Command: strings.Split(command, " "),
						},
					},
					RestartPolicy: apiv1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backoffLimit,
		},
	}
	_, err := clientset.BatchV1().Jobs("scheduler").Create(context.TODO(), newJob, metav1.CreateOptions{})
	if err != nil {
		panic(err.Error())
	}
}

func getJobLogs(clientset *kubernetes.Clientset, name string, out io.WriteCloser, wg *sync.WaitGroup) {
    defer fmt.Println("done with getting logs")
    defer wg.Done()
    defer out.Close()
	pods, err := clientset.CoreV1().Pods("scheduler").List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", name),
	})
	if err != nil {
		panic(err)
	}
	if len(pods.Items) == 0 {
		panic(errors.New("pod not found"))
	}

	podLogRequest := clientset.CoreV1().Pods("scheduler").GetLogs(pods.Items[0].Name, &apiv1.PodLogOptions{
		Follow: true,
	})
	logsCtx := context.Background()
	logsCtx, done := context.WithCancel(logsCtx)
	defer done()

	stream, err := podLogRequest.Stream(logsCtx)
	if err != nil {
		panic(err)
	}

	for {
        buf := make([]byte, 20)
		numBytes, err := stream.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		out.Write(buf[:numBytes])
        job, err := clientset.BatchV1().Jobs("scheduler").Get(context.TODO(), name, metav1.GetOptions{})
        if err != nil {
            panic(err)
        }
        if numBytes == 0 && (job.Status.Failed != 0 || job.Status.Succeeded != 0) {
            // job has completed
            break
        }
        time.Sleep(time.Second)
	}
}
