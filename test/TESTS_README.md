## Platform-Apis Tests

### Prerequisites
Go requires import of one library which is actually a folder in entigo-infralib repo.
Clone infralib repo and use symlink to mock folder existence in platform-apis.

```
ln -s <full path to cloned entigo-infralib repo>/common <full path to cloned platform-apis repo>/common
```

### Test Suite

Test Suite consists of 2 test steps:
1. Static tests (code tests). Include:
    * Resources rendering using Crossplane CLI - allows you to test if all resources rendered in right order
    and if all resources metadata and specs fields populated properly.
    * Kyverno policies check Kyverno CLI - allows you to test if all policies implemented properly.
    *  Go tests for functions testing.
2. End-2-end (live platform-apis package) tests:
    * Running in AWS cluster using entigo-infralib test image.

All test written in Go (v. 1.25.7)

### Test Writing Guides